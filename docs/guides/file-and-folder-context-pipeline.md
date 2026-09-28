# File & Folder Context Pipeline — Complete Implementation Reference

This document provides a complete, code-backed specification for how files and folders are processed and delivered to the LLM. Every section includes the exact implementation from the source code so an agent can recreate this system from scratch.

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Entry Points: How Content Enters the Pipeline](#2-entry-points)
3. [Path Extraction & Resolution](#3-path-extraction--resolution)
4. [File Reading: Core Implementation](#4-file-reading-core-implementation)
5. [Size Gates & Token Validation](#5-size-gates--token-validation)
6. [Folder/Directory Processing](#6-folderdirectory-processing)
7. [Attachment Generation & Truncation Fallback](#7-attachment-generation--truncation-fallback)
8. [Deduplication: Avoiding Re-sending Content](#8-deduplication)
9. [Serialization: Content → Line-Numbered Text](#9-serialization-content--line-numbered-text)
10. [Attachment → API Message Conversion](#10-attachment--api-message-conversion)
11. [System & User Context Assembly](#11-system--user-context-assembly)
12. [Query Loop: Final Message Assembly](#12-query-loop-final-message-assembly)
13. [Final API Wire Format](#13-final-api-wire-format)
14. [Context Window Management](#14-context-window-management)
15. [Special File Types: Images, PDFs, Notebooks](#15-special-file-types)
16. [Complete Limits Reference](#16-complete-limits-reference)

---

## 1. Architecture Overview

```
User Input ("fix @src/foo.ts")
         │
         ▼
┌─────────────────────────────────────────┐
│  processUserInput()                     │
│  src/utils/processUserInput/            │
│  processUserInput.ts                    │
│                                         │
│  1. Parse input                         │
│  2. Extract @mentions                   │
│  3. Load attachments (files/dirs)       │
│  4. Build UserMessage + Attachments     │
└─────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│  query() — The Query Loop               │
│  src/query.ts                           │
│                                         │
│  1. Apply tool result budget            │
│  2. Snip/microcompact/collapse          │
│  3. Prepend user context (CLAUDE.md)    │
│  4. Call model API                      │
│  5. Process tool results                │
│  6. Loop if tools were used             │
└─────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│  queryModelWithStreaming()               │
│  src/services/api/claude.ts             │
│                                         │
│  1. normalizeMessagesForAPI()           │
│  2. Build system prompt blocks          │
│  3. Add cache breakpoints              │
│  4. beta.messages.create()             │
└─────────────────────────────────────────┘
```

Two paths exist for file content to reach the LLM:

| Path | Trigger | Code Entry |
|------|---------|-----------|
| **@-mention attachment** | User types `@path` | `processAtMentionedFiles()` in `src/utils/attachments.ts` |
| **Read tool call** | Model calls the `Read` tool | `FileReadTool.call()` in `src/tools/FileReadTool/FileReadTool.ts` |

Both paths use the same core reader (`readFileInRange` in `src/utils/readFileInRange.ts`) and the same token validation logic.

---

## 2. Entry Points

### 2.1 User Input Processing

When the user submits a prompt, `processUserInput` orchestrates attachment loading:

```typescript
// src/utils/processUserInput/processUserInput.ts (lines 496-513)
const attachmentMessages = shouldExtractAttachments
  ? await toArray(
      getAttachmentMessages(
        inputString,
        context,
        ideSelection ?? null,
        [],
        messages,
        querySource,
      ),
    )
  : []
```

The `getAttachmentMessages` function internally calls `getAttachments()`, which dispatches multiple attachment producers in parallel:

```typescript
// src/utils/attachments.ts (lines 772-815 conceptually)
const userInputAttachments = input
  ? [
      maybe('at_mentioned_files', () =>
        processAtMentionedFiles(input, context),
      ),
      maybe('mcp_resources', () =>
        processMcpResourceAttachments(input, context),
      ),
      // ... other attachment types
    ]
  : []
```

### 2.2 The Final User Message

After attachment processing, the user prompt becomes a `UserMessage` plus attachment messages:

```typescript
// src/utils/processUserInput/processTextPrompt.ts (lines 89-98)
const userMessage = createUserMessage({
  content: input,
  uuid,
  permissionMode,
  isMeta: isMeta || undefined,
})

return {
  messages: [userMessage, ...attachmentMessages],
  shouldQuery: true,
}
```

---

## 3. Path Extraction & Resolution

### 3.1 Regex-Based @-Mention Extraction

The system uses two regex patterns to extract file paths from user input:

```typescript
// src/utils/attachments.ts (lines 2757-2789)
export function extractAtMentionedFiles(content: string): string[] {
  // Two patterns: quoted paths and regular paths
  const quotedAtMentionRegex = /(^|\s)@"([^"]+)"/g
  const regularAtMentionRegex = /(^|\s)@([^\s]+)\b/g

  const quotedMatches: string[] = []
  const regularMatches: string[] = []

  // Extract quoted mentions first (skip agent mentions like @"code-reviewer (agent)")
  let match
  while ((match = quotedAtMentionRegex.exec(content)) !== null) {
    if (match[2] && !match[2].endsWith(' (agent)')) {
      quotedMatches.push(match[2])
    }
  }

  // Extract regular mentions
  const regularMatchArray = content.match(regularAtMentionRegex) || []
  regularMatchArray.forEach(match => {
    const filename = match.slice(match.indexOf('@') + 1)
    if (!filename.startsWith('"')) {
      regularMatches.push(filename)
    }
  })

  return uniq([...quotedMatches, ...regularMatches])
}
```

### 3.2 Line Range Parsing

Paths can include line ranges like `@file.txt#L10-20`. The `parseAtMentionedFileLines()` function extracts these, converting them to `offset` and `limit` parameters.

### 3.3 Path Resolution

All paths are normalized through `expandPath()` from `src/utils/path.ts`, which handles:
- Tilde expansion (`~/`)
- Relative path resolution
- Windows path separators
- Whitespace trimming

---

## 4. File Reading: Core Implementation

### 4.1 The `readFileInRange` Function

This is the single reader underlying all file content access:

```typescript
// src/utils/readFileInRange.ts (lines 73-122)
export async function readFileInRange(
  filePath: string,
  offset = 0,
  maxLines?: number,
  maxBytes?: number,
  signal?: AbortSignal,
  options?: { truncateOnByteLimit?: boolean },
): Promise<ReadFileRangeResult> {
  signal?.throwIfAborted()
  const truncateOnByteLimit = options?.truncateOnByteLimit ?? false

  const stats = await fsStat(filePath)

  if (stats.isDirectory()) {
    throw new Error(
      `EISDIR: illegal operation on a directory, read '${filePath}'`,
    )
  }

  // Fast path: regular files under 10 MB
  if (stats.isFile() && stats.size < FAST_PATH_MAX_SIZE) {
    if (
      !truncateOnByteLimit &&
      maxBytes !== undefined &&
      stats.size > maxBytes
    ) {
      throw new FileTooLargeError(stats.size, maxBytes)
    }

    const text = await readFile(filePath, { encoding: 'utf8', signal })
    return readFileInRangeFast(
      text,
      stats.mtimeMs,
      offset,
      maxLines,
      truncateOnByteLimit ? maxBytes : undefined,
    )
  }

  // Streaming path: files >= 10 MB, pipes, devices
  return readFileInRangeStreaming(
    filePath, offset, maxLines, maxBytes, truncateOnByteLimit, signal,
  )
}
```

### 4.2 Fast Path (< 10 MB Files)

```typescript
// src/utils/readFileInRange.ts (lines 44, 128-194)
const FAST_PATH_MAX_SIZE = 10 * 1024 * 1024 // 10 MB

function readFileInRangeFast(
  raw: string,
  mtimeMs: number,
  offset: number,
  maxLines: number | undefined,
  truncateAtBytes: number | undefined,
): ReadFileRangeResult {
  const endLine = maxLines !== undefined ? offset + maxLines : Infinity

  // Strip BOM
  const text = raw.charCodeAt(0) === 0xfeff ? raw.slice(1) : raw

  // Split lines, strip \r, select range
  const selectedLines: string[] = []
  let lineIndex = 0
  let startPos = 0
  let newlinePos: number
  let selectedBytes = 0
  let truncatedByBytes = false

  function tryPush(line: string): boolean {
    if (truncateAtBytes !== undefined) {
      const sep = selectedLines.length > 0 ? 1 : 0
      const nextBytes = selectedBytes + sep + Buffer.byteLength(line)
      if (nextBytes > truncateAtBytes) {
        truncatedByBytes = true
        return false
      }
      selectedBytes = nextBytes
    }
    selectedLines.push(line)
    return true
  }

  while ((newlinePos = text.indexOf('\n', startPos)) !== -1) {
    if (lineIndex >= offset && lineIndex < endLine && !truncatedByBytes) {
      let line = text.slice(startPos, newlinePos)
      if (line.endsWith('\r')) {
        line = line.slice(0, -1)
      }
      tryPush(line)
    }
    lineIndex++
    startPos = newlinePos + 1
  }

  // Final fragment (no trailing newline)
  if (lineIndex >= offset && lineIndex < endLine && !truncatedByBytes) {
    let line = text.slice(startPos)
    if (line.endsWith('\r')) {
      line = line.slice(0, -1)
    }
    tryPush(line)
  }
  lineIndex++

  const content = selectedLines.join('\n')
  return {
    content,
    lineCount: selectedLines.length,
    totalLines: lineIndex,
    totalBytes: Buffer.byteLength(text, 'utf8'),
    readBytes: Buffer.byteLength(content, 'utf8'),
    mtimeMs,
    ...(truncatedByBytes ? { truncatedByBytes: true } : {}),
  }
}
```

### 4.3 Streaming Path (>= 10 MB Files)

For large files, the streaming path uses `createReadStream` with a 512 KB highWaterMark. Only lines within the requested range are accumulated — lines outside are counted but discarded, preventing OOM on huge files:

```typescript
// src/utils/readFileInRange.ts (lines 344-383)
function readFileInRangeStreaming(
  filePath: string,
  offset: number,
  maxLines: number | undefined,
  maxBytes: number | undefined,
  truncateOnByteLimit: boolean,
  signal?: AbortSignal,
): Promise<ReadFileRangeResult> {
  return new Promise((resolve, reject) => {
    const state: StreamState = {
      stream: createReadStream(filePath, {
        encoding: 'utf8',
        highWaterMark: 512 * 1024,
        ...(signal ? { signal } : undefined),
      }),
      offset,
      endLine: maxLines !== undefined ? offset + maxLines : Infinity,
      maxBytes,
      truncateOnByteLimit,
      resolve,
      totalBytesRead: 0,
      selectedBytes: 0,
      truncatedByBytes: false,
      currentLineIndex: 0,
      selectedLines: [],
      partial: '',
      isFirstChunk: true,
      resolveMtime: () => {},
      mtimeReady: null as unknown as Promise<number>,
    }
    state.mtimeReady = new Promise<number>(r => {
      state.resolveMtime = r
    })

    state.stream.once('open', streamOnOpen.bind(state))
    state.stream.on('data', streamOnData.bind(state))
    state.stream.once('end', streamOnEnd.bind(state))
    state.stream.once('error', reject)
  })
}
```

The streaming `onData` handler checks bytes consumed and throws `FileTooLargeError` when `maxBytes` is exceeded (unless `truncateOnByteLimit: true`):

```typescript
// src/utils/readFileInRange.ts (lines 224-243)
function streamOnData(this: StreamState, chunk: string): void {
  if (this.isFirstChunk) {
    this.isFirstChunk = false
    if (chunk.charCodeAt(0) === 0xfeff) {
      chunk = chunk.slice(1)
    }
  }

  this.totalBytesRead += Buffer.byteLength(chunk)
  if (
    !this.truncateOnByteLimit &&
    this.maxBytes !== undefined &&
    this.totalBytesRead > this.maxBytes
  ) {
    this.stream.destroy(
      new FileTooLargeError(this.totalBytesRead, this.maxBytes),
    )
    return
  }
  // ... line accumulation logic
}
```

### 4.4 The Return Type

```typescript
// src/utils/readFileInRange.ts (lines 46-55)
export type ReadFileRangeResult = {
  content: string
  lineCount: number
  totalLines: number
  totalBytes: number
  readBytes: number
  mtimeMs: number
  truncatedByBytes?: boolean
}
```

---

## 5. Size Gates & Token Validation

### 5.1 File Size Limit (256 KB)

The byte limit constant:

```typescript
// src/utils/file.ts (line 48)
export const MAX_OUTPUT_SIZE = 0.25 * 1024 * 1024 // 0.25MB in bytes
```

The limits configuration with precedence chain:

```typescript
// src/tools/FileReadTool/limits.ts (lines 18, 53-92)
export const DEFAULT_MAX_OUTPUT_TOKENS = 25000

export const getDefaultFileReadingLimits = memoize((): FileReadingLimits => {
  const override =
    getFeatureValue_CACHED_MAY_BE_STALE<Partial<FileReadingLimits> | null>(
      'tengu_amber_wren',
      {},
    )

  const maxSizeBytes =
    typeof override?.maxSizeBytes === 'number' &&
    Number.isFinite(override.maxSizeBytes) &&
    override.maxSizeBytes > 0
      ? override.maxSizeBytes
      : MAX_OUTPUT_SIZE

  const envMaxTokens = getEnvMaxTokens()
  const maxTokens =
    envMaxTokens ??
    (typeof override?.maxTokens === 'number' &&
    Number.isFinite(override.maxTokens) &&
    override.maxTokens > 0
      ? override.maxTokens
      : DEFAULT_MAX_OUTPUT_TOKENS)

  return { maxSizeBytes, maxTokens, includeMaxSizeInPrompt, targetedRangeNudge }
})
```

**Precedence for `maxTokens`:** `CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS` env var → GrowthBook flag → `25000`

**Precedence for `maxSizeBytes`:** GrowthBook override → `MAX_OUTPUT_SIZE` (256 KB)

### 5.2 How Size Check is Applied in the Read Tool

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 1019-1028)
// --- Text file (single async read via readFileInRange) ---
const lineOffset = offset === 0 ? 0 : offset - 1
const { content, lineCount, totalLines, totalBytes, readBytes, mtimeMs } =
  await readFileInRange(
    resolvedFilePath,
    lineOffset,
    limit,
    limit === undefined ? maxSizeBytes : undefined,  // KEY: only passes maxBytes when NO limit
    context.abortController.signal,
  )

await validateContentTokens(content, ext, maxTokens)
```

**Critical behavior:** When the user provides a `limit` (line count), `maxBytes` is NOT passed — the byte pre-check is skipped. Only the 25K token cap applies to the returned slice. This means you can read a slice of a multi-GB file safely.

### 5.3 Token Validation — Two-Stage Check

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 755-772)
async function validateContentTokens(
  content: string,
  ext: string,
  maxTokens?: number,
): Promise<void> {
  const effectiveMaxTokens =
    maxTokens ?? getDefaultFileReadingLimits().maxTokens

  // Stage 1: Fast heuristic (no API call)
  const tokenEstimate = roughTokenCountEstimationForFileType(content, ext)
  if (!tokenEstimate || tokenEstimate <= effectiveMaxTokens / 4) return

  // Stage 2: Precise count via API (only if estimate > 25% of limit)
  const tokenCount = await countTokensWithAPI(content)
  const effectiveCount = tokenCount ?? tokenEstimate

  if (effectiveCount > effectiveMaxTokens) {
    throw new MaxFileReadTokenExceededError(effectiveCount, effectiveMaxTokens)
  }
}
```

### 5.4 Token Estimation Heuristics

```typescript
// src/services/tokenEstimation.ts (lines 203-242)
export function roughTokenCountEstimation(
  content: string,
  bytesPerToken: number = 4,
): number {
  return Math.round(content.length / bytesPerToken)
}

export function bytesPerTokenForFileType(fileExtension: string): number {
  switch (fileExtension) {
    case 'json':
    case 'jsonl':
    case 'jsonc':
      return 2   // JSON is token-dense (single-char tokens like {, }, :, etc.)
    default:
      return 4   // Code/text: ~4 bytes per token
  }
}

export function roughTokenCountEstimationForFileType(
  content: string,
  fileExtension: string,
): number {
  return roughTokenCountEstimation(
    content,
    bytesPerTokenForFileType(fileExtension),
  )
}
```

### 5.5 The Error Types

```typescript
// src/utils/readFileInRange.ts (lines 57-66)
export class FileTooLargeError extends Error {
  constructor(
    public sizeInBytes: number,
    public maxSizeBytes: number,
  ) {
    super(
      `File content (${formatFileSize(sizeInBytes)}) exceeds maximum allowed size (${formatFileSize(maxSizeBytes)}). Use offset and limit parameters to read specific portions of the file, or search for specific content instead of reading the whole file.`,
    )
    this.name = 'FileTooLargeError'
  }
}
```

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 175-184, referenced)
export class MaxFileReadTokenExceededError extends Error {
  constructor(
    public tokenCount: number,
    public maxTokens: number,
  ) {
    super(
      `File content (${tokenCount} tokens) exceeds maximum allowed tokens (${maxTokens}). Use offset and limit parameters to read specific portions of the file, or search for specific content instead of reading the whole file.`,
    )
    this.name = 'MaxFileReadTokenExceededError'
  }
}
```

---

## 6. Folder/Directory Processing

### 6.1 Directory Detection & Shallow Listing

When an `@`-mention resolves to a directory, the system does a **single-level `readdir`** — it is NOT recursive:

```typescript
// src/utils/attachments.ts (lines 1914-1941)
// Check if it's a directory
try {
  const stats = await stat(absoluteFilename)
  if (stats.isDirectory()) {
    try {
      const entries = await readdir(absoluteFilename, {
        withFileTypes: true,
      })
      const MAX_DIR_ENTRIES = 1000
      const truncated = entries.length > MAX_DIR_ENTRIES
      const names = entries.slice(0, MAX_DIR_ENTRIES).map(e => e.name)
      if (truncated) {
        names.push(
          `\u2026 and ${entries.length - MAX_DIR_ENTRIES} more entries`,
        )
      }
      const stdout = names.join('\n')
      logEvent('tengu_at_mention_extracting_directory_success', {})

      return {
        type: 'directory' as const,
        path: absoluteFilename,
        content: stdout,
        displayPath: relative(getCwd(), absoluteFilename),
      }
    } catch {
      return null
    }
  }
} catch {
  // If stat fails, continue with file logic
}
```

**Key constraints:**
- Only immediate children (one level)
- Maximum 1000 entries
- Only entry names — no file contents, no sizes, no metadata
- Files and directories are not distinguished in the output

### 6.2 What the LLM Receives for Directories

The directory is converted into a **synthetic `ls` Bash command result** (see Section 10).

### 6.3 No Recursive Reading Exists

The system intentionally does NOT provide recursive folder reading. The model must use tools iteratively:
1. `@folder/` → get top-level listing
2. Model calls `Glob` to find specific file patterns
3. Model calls `Read` on individual files
4. Model calls `Grep` to search within files

---

## 7. Attachment Generation & Truncation Fallback

### 7.1 The Full Pipeline for File Attachments

```typescript
// src/utils/attachments.ts (lines 3030-3199)
async function generateFileAttachment(
  filename, toolUseContext, successEventName, errorEventName, mode, options
) {
  const { offset, limit } = options ?? {}

  // GATE 1: Permission check
  if (isFileReadDenied(filename, appState.toolPermissionContext)) {
    return null
  }

  // GATE 2: Pre-read size check (@ mentions only, non-PDF)
  if (
    mode === 'at-mention' &&
    !isFileWithinReadSizeLimit(filename, getDefaultFileReadingLimits().maxSizeBytes)
  ) {
    const ext = parse(filename).ext.toLowerCase()
    if (!isPDFExtension(ext)) {
      try {
        const stats = await getFsImplementation().stat(filename)
        logEvent('tengu_attachment_file_too_large', { size_bytes: stats.size, mode })
        return null  // File silently rejected
      } catch { }
    }
  }

  // GATE 3: Large PDF → reference only
  if (mode === 'at-mention') {
    const pdfRef = await tryGetPDFReference(filename)
    if (pdfRef) return pdfRef
  }

  // GATE 4: Already in context (dedup)
  const existingFileState = toolUseContext.readFileState.get(filename)
  if (existingFileState && mode === 'at-mention') {
    const mtimeMs = await getFileModificationTimeAsync(filename)
    if (existingFileState.timestamp <= mtimeMs && mtimeMs === existingFileState.timestamp) {
      return {
        type: 'already_read_file',
        filename,
        displayPath: relative(getCwd(), filename),
        content: { type: 'text', file: { ... } },
      }
    }
  }

  // ATTEMPT: Full file read
  try {
    const result = await FileReadTool.call(fileInput, toolUseContext)
    return { type: 'file', filename, content: result.data, displayPath: ... }
  } catch (error) {
    // FALLBACK: On size/token overflow, read first 2000 lines
    if (
      error instanceof MaxFileReadTokenExceededError ||
      error instanceof FileTooLargeError
    ) {
      return await readTruncatedFile()
    }
    throw error
  }
}
```

### 7.2 The Truncation Fallback

```typescript
// src/utils/attachments.ts (lines 3148-3168)
async function readTruncatedFile() {
  if (mode === 'compact') {
    return {
      type: 'compact_file_reference',
      filename,
      displayPath: relative(getCwd(), filename),
    }
  }

  try {
    // Read only the first MAX_LINES_TO_READ lines
    const truncatedInput = {
      file_path: filename,
      offset: offset ?? 1,
      limit: MAX_LINES_TO_READ,  // 2000
    }
    const result = await FileReadTool.call(truncatedInput, toolUseContext)

    return {
      type: 'file' as const,
      filename,
      content: result.data,
      truncated: true,  // This flag triggers a meta-message to the model
      displayPath: relative(getCwd(), filename),
    }
  } catch {
    return null
  }
}
```

The `MAX_LINES_TO_READ` constant:

```typescript
// src/tools/FileReadTool/prompt.ts (line 10)
export const MAX_LINES_TO_READ = 2000
```

---

## 8. Deduplication

### 8.1 Read Tool Deduplication

When the model reads the same file+range again and it hasn't changed, a stub is returned:

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 523-572)
// Dedup: if we've already read this exact range and the file hasn't
// changed on disk, return a stub instead of re-sending the full content.
const existingState = dedupKillswitch
  ? undefined
  : readFileState.get(fullFilePath)

if (
  existingState &&
  !existingState.isPartialView &&
  existingState.offset !== undefined
) {
  const rangeMatch =
    existingState.offset === offset && existingState.limit === limit
  if (rangeMatch) {
    try {
      const mtimeMs = await getFileModificationTimeAsync(fullFilePath)
      if (mtimeMs === existingState.timestamp) {
        return {
          data: {
            type: 'file_unchanged' as const,
            file: { filePath: file_path },
          },
        }
      }
    } catch {
      // stat failed — fall through to full read
    }
  }
}
```

### 8.2 The Stub Message Sent to the Model

```typescript
// src/tools/FileReadTool/prompt.ts (lines 7-8)
export const FILE_UNCHANGED_STUB =
  'File unchanged since last read. The content from the earlier Read tool_result in this conversation is still current — refer to that instead of re-reading.'
```

---

## 9. Serialization: Content → Line-Numbered Text

### 9.1 Line Number Formatting

All text file content gets `cat -n` style line numbers before being sent to the model:

```typescript
// src/utils/file.ts (lines 290-319)
export function addLineNumbers({
  content,
  startLine,  // 1-indexed
}: {
  content: string
  startLine: number
}): string {
  if (!content) {
    return ''
  }

  const lines = content.split(/\r?\n/)

  if (isCompactLinePrefixEnabled()) {
    // Compact format: "1\tcontent"
    return lines
      .map((line, index) => `${index + startLine}\t${line}`)
      .join('\n')
  }

  // Standard format: "     1→content" (6-char padded with arrow separator)
  return lines
    .map((line, index) => {
      const numStr = String(index + startLine)
      if (numStr.length >= 6) {
        return `${numStr}→${line}`
      }
      return `${numStr.padStart(6, ' ')}→${line}`
    })
    .join('\n')
}
```

### 9.2 How Line Numbers Are Applied in the Tool Result

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 692-715)
case 'text': {
  let content: string

  if (data.file.content) {
    content =
      memoryFileFreshnessPrefix(data) +
      formatFileLines(data.file) +           // ← addLineNumbers() is called here
      (shouldIncludeFileReadMitigation()
        ? CYBER_RISK_MITIGATION_REMINDER
        : '')
  } else {
    content =
      data.file.totalLines === 0
        ? '<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>'
        : `<system-reminder>Warning: the file exists but is shorter than the provided offset (${data.file.startLine}). The file has ${data.file.totalLines} lines.</system-reminder>`
  }

  return {
    tool_use_id: toolUseID,
    type: 'tool_result',
    content,
  }
}
```

Where `formatFileLines` simply calls:

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 724-727)
function formatFileLines(file: { content: string; startLine: number }): string {
  return addLineNumbers(file)
}
```

---

## 10. Attachment → API Message Conversion

### 10.1 The Normalization Dispatch

During API message assembly, internal `AttachmentMessage` objects are converted:

```typescript
// src/utils/messages.ts (lines 2269-2290)
case 'attachment': {
  const rawAttachmentMessage = normalizeAttachmentForAPI(
    message.attachment,
  )
  const attachmentMessage = checkStatsigFeatureGate_CACHED_MAY_BE_STALE(
    'tengu_chair_sermon',
  )
    ? rawAttachmentMessage.map(ensureSystemReminderWrap)
    : rawAttachmentMessage

  // If the last message is also a user message, merge them
  const lastMessage = last(result)
  if (lastMessage?.type === 'user') {
    result[result.length - 1] = attachmentMessage.reduce(
      (p, c) => mergeUserMessagesAndToolResults(p, c),
      lastMessage,
    )
    return
  }

  result.push(...attachmentMessage)
  return
}
```

### 10.2 Directory → Synthetic `ls` Tool Call

```typescript
// src/utils/messages.ts (lines 3525-3536)
case 'directory': {
  return wrapMessagesInSystemReminder([
    createToolUseMessage(BashTool.name, {
      command: `ls ${quote([attachment.path])}`,
      description: `Lists files in ${attachment.path}`,
    }),
    createToolResultMessage(BashTool, {
      stdout: attachment.content,
      stderr: '',
      interrupted: false,
    }),
  ])
}
```

### 10.3 File → Synthetic `Read` Tool Call + Result

```typescript
// src/utils/messages.ts (lines 3545-3570)
case 'file': {
  const fileContent = attachment.content as FileReadToolOutput
  switch (fileContent.type) {
    case 'text': {
      return wrapMessagesInSystemReminder([
        createToolUseMessage(FileReadTool.name, {
          file_path: attachment.filename,
        }),
        createToolResultMessage(FileReadTool, fileContent),
        ...(attachment.truncated
          ? [
              createUserMessage({
                content: `Note: The file ${attachment.filename} was too large and has been truncated to the first ${MAX_LINES_TO_READ} lines. Don't tell the user about this truncation. Use ${FileReadTool.name} to read more of the file if you need.`,
                isMeta: true,
              }),
            ]
          : []),
      ])
    }
  }
}
```

### 10.4 How Synthetic Tool Messages Are Built

```typescript
// src/utils/messages.ts (lines 4325-4333)
function createToolUseMessage(
  toolName: string,
  input: { [key: string]: string | number },
): UserMessage {
  return createUserMessage({
    content: `Called the ${toolName} tool with the following input: ${jsonStringify(input)}`,
    isMeta: true,
  })
}
```

```typescript
// src/utils/messages.ts (lines 4290-4323)
function createToolResultMessage(tool, toolUseResult): UserMessage {
  try {
    const result = tool.mapToolResultToToolResultBlockParam(toolUseResult, '1')

    // If the result contains image content blocks, preserve them as is
    if (
      Array.isArray(result.content) &&
      result.content.some(block => block.type === 'image')
    ) {
      return createUserMessage({
        content: result.content as ContentBlockParam[],
        isMeta: true,
      })
    }

    // For string content, use raw string (avoid escaping \n→\\n)
    const contentStr =
      typeof result.content === 'string'
        ? result.content
        : jsonStringify(result.content)
    return createUserMessage({
      content: `Result of calling the ${tool.name} tool:\n${contentStr}`,
      isMeta: true,
    })
  } catch {
    return createUserMessage({
      content: `Result of calling the ${tool.name} tool: Error`,
      isMeta: true,
    })
  }
}
```

### 10.5 Other Attachment Types

**PDF reference (large PDF):**
```typescript
// src/utils/messages.ts (lines 3600-3611)
case 'pdf_reference': {
  return wrapMessagesInSystemReminder([
    createUserMessage({
      content:
        `PDF file: ${attachment.filename} (${attachment.pageCount} pages, ${formatFileSize(attachment.fileSize)}). ` +
        `This PDF is too large to read all at once. You MUST use the ${FILE_READ_TOOL_NAME} tool with the pages parameter ` +
        `to read specific page ranges (e.g., pages: "1-5"). Maximum 20 pages per request.`,
      isMeta: true,
    }),
  ])
}
```

**IDE selection:**
```typescript
// src/utils/messages.ts (lines 3613-3627)
case 'selected_lines_in_ide': {
  const maxSelectionLength = 2000
  const content =
    attachment.content.length > maxSelectionLength
      ? attachment.content.substring(0, maxSelectionLength) +
        '\n... (truncated)'
      : attachment.content

  return wrapMessagesInSystemReminder([
    createUserMessage({
      content: `The user selected the lines ${attachment.lineStart} to ${attachment.lineEnd} from ${attachment.filename}:\n${content}\n\nThis may or may not be related to the current task.`,
      isMeta: true,
    }),
  ])
}
```

**Already-read file (dedup at attachment level):**
```typescript
// src/utils/messages.ts (lines 4252-4261, referenced)
case 'already_read_file':
  return []  // Produces NO API messages
```

---

## 11. System & User Context Assembly

### 11.1 System Context (Git Status)

```typescript
// src/context.ts (lines 116-150)
export const getSystemContext = memoize(
  async (): Promise<{ [k: string]: string }> => {
    const gitStatus =
      isEnvTruthy(process.env.CLAUDE_CODE_REMOTE) ||
      !shouldIncludeGitInstructions()
        ? null
        : await getGitStatus()

    return {
      ...(gitStatus && { gitStatus }),
      ...(feature('BREAK_CACHE_COMMAND') && injection
        ? { cacheBreaker: `[CACHE_BREAKER: ${injection}]` }
        : {}),
    }
  },
)
```

Git status is capped at 2K characters (truncated in `getGitStatus()`).

### 11.2 User Context (CLAUDE.md + Date)

```typescript
// src/context.ts (lines 155-189)
export const getUserContext = memoize(
  async (): Promise<{ [k: string]: string }> => {
    const shouldDisableClaudeMd =
      isEnvTruthy(process.env.CLAUDE_CODE_DISABLE_CLAUDE_MDS) ||
      (isBareMode() && getAdditionalDirectoriesForClaudeMd().length === 0)

    const claudeMd = shouldDisableClaudeMd
      ? null
      : getClaudeMds(filterInjectedMemoryFiles(await getMemoryFiles()))

    return {
      ...(claudeMd && { claudeMd }),
      currentDate: `Today's date is ${getLocalISODate()}.`,
    }
  },
)
```

### 11.3 How System Context is Appended to System Prompt

```typescript
// src/utils/api.ts (lines 437-446)
export function appendSystemContext(
  systemPrompt: SystemPrompt,
  context: { [k: string]: string },
): string[] {
  return [
    ...systemPrompt,
    Object.entries(context)
      .map(([key, value]) => `${key}: ${value}`)
      .join('\n'),
  ].filter(Boolean)
}
```

### 11.4 How User Context is Prepended as First Message

```typescript
// src/utils/api.ts (lines 449-474)
export function prependUserContext(
  messages: Message[],
  context: { [k: string]: string },
): Message[] {
  if (Object.entries(context).length === 0) {
    return messages
  }

  return [
    createUserMessage({
      content: `<system-reminder>\nAs you answer the user's questions, you can use the following context:\n${Object.entries(
        context,
      )
        .map(([key, value]) => `# ${key}\n${value}`)
        .join('\n')}

      IMPORTANT: this context may or may not be relevant to your tasks. You should not respond to this context unless it is highly relevant to your task.\n</system-reminder>\n`,
      isMeta: true,
    }),
    ...messages,
  ]
}
```

---

## 12. Query Loop: Final Message Assembly

### 12.1 Pre-API Processing Pipeline

Each loop iteration applies these transformations **in order** before calling the API:

```typescript
// src/query.ts (lines 365-460)
let messagesForQuery = [...getMessagesAfterCompactBoundary(messages)]

// Stage 1: Tool result budget enforcement
messagesForQuery = await applyToolResultBudget(
  messagesForQuery,
  toolUseContext.contentReplacementState,
  persistReplacements ? records => void recordContentReplacement(...) : undefined,
  new Set(
    toolUseContext.options.tools
      .filter(t => !Number.isFinite(t.maxResultSizeChars))
      .map(t => t.name),
  ),
)

// Stage 2: History snip (remove oldest turns)
if (feature('HISTORY_SNIP')) {
  const snipResult = snipModule!.snipCompactIfNeeded(messagesForQuery)
  messagesForQuery = snipResult.messages
  snipTokensFreed = snipResult.tokensFreed
}

// Stage 3: Microcompact (compress/prune tool results)
const microcompactResult = await deps.microcompact(
  messagesForQuery, toolUseContext, querySource,
)
messagesForQuery = microcompactResult.messages

// Stage 4: Context collapse (collapse old tool exchanges into summaries)
if (feature('CONTEXT_COLLAPSE') && contextCollapse) {
  const collapseResult = await contextCollapse.applyCollapsesIfNeeded(
    messagesForQuery, toolUseContext, querySource,
  )
  messagesForQuery = collapseResult.messages
}

// Stage 5: Build full system prompt (append git status etc.)
const fullSystemPrompt = asSystemPrompt(
  appendSystemContext(systemPrompt, systemContext),
)

// Stage 6: Autocompact (summarize conversation if near limit)
const { compactionResult } = await deps.autocompact(
  messagesForQuery, toolUseContext, { systemPrompt, userContext, ... },
)
```

### 12.2 The API Call

```typescript
// src/query.ts (lines 659-661)
for await (const message of deps.callModel({
  messages: prependUserContext(messagesForQuery, userContext),
  systemPrompt: fullSystemPrompt,
  thinkingConfig: toolUseContext.options.thinkingConfig,
  tools: toolUseContext.options.tools,
  signal: toolUseContext.abortController.signal,
  options: { model: currentModel, ... },
})) {
  // ... process streaming response
}
```

---

## 13. Final API Wire Format

### 13.1 System Prompt → TextBlockParam[]

```typescript
// src/services/api/claude.ts (lines 3213-3237)
export function buildSystemPromptBlocks(
  systemPrompt: SystemPrompt,
  enablePromptCaching: boolean,
  options?: {
    skipGlobalCacheForSystemPrompt?: boolean
    querySource?: QuerySource
  },
): TextBlockParam[] {
  return splitSysPromptPrefix(systemPrompt, {
    skipGlobalCacheForSystemPrompt: options?.skipGlobalCacheForSystemPrompt,
  }).map(block => {
    return {
      type: 'text' as const,
      text: block.text,
      ...(enablePromptCaching &&
        block.cacheScope !== null && {
          cache_control: getCacheControl({
            scope: block.cacheScope,
            querySource: options?.querySource,
          }),
        }),
    }
  })
}
```

### 13.2 System Prompt Assembly Layers

```typescript
// src/services/api/claude.ts (lines 1358-1379)
systemPrompt = asSystemPrompt(
  [
    getAttributionHeader(fingerprint),
    getCLISyspromptPrefix({
      isNonInteractive: options.isNonInteractiveSession,
      hasAppendSystemPrompt: options.hasAppendSystemPrompt,
    }),
    ...systemPrompt,
    ...(advisorModel ? [ADVISOR_TOOL_INSTRUCTIONS] : []),
    ...(injectChromeHere ? [CHROME_TOOL_SEARCH_INSTRUCTIONS] : []),
  ].filter(Boolean),
)

const system = buildSystemPromptBlocks(systemPrompt, enablePromptCaching, {
  skipGlobalCacheForSystemPrompt: needsToolBasedCacheMarker,
  querySource: options.querySource,
})
```

### 13.3 Message Conversion to API Format

```typescript
// src/services/api/claude.ts (lines 588-631)
export function userMessageToMessageParam(
  message: UserMessage,
  addCache = false,
  enablePromptCaching: boolean,
  querySource?: QuerySource,
): MessageParam {
  if (addCache) {
    if (typeof message.message.content === 'string') {
      return {
        role: 'user',
        content: [
          {
            type: 'text',
            text: message.message.content,
            ...(enablePromptCaching && {
              cache_control: getCacheControl({ querySource }),
            }),
          },
        ],
      }
    } else {
      return {
        role: 'user',
        content: message.message.content.map((_, i) => ({
          ..._,
          ...(i === message.message.content.length - 1
            ? enablePromptCaching
              ? { cache_control: getCacheControl({ querySource }) }
              : {}
            : {}),
        })),
      }
    }
  }
  return {
    role: 'user',
    content: Array.isArray(message.message.content)
      ? [...message.message.content]
      : message.message.content,
  }
}
```

### 13.4 The Final API Request Shape

```typescript
// src/services/api/claude.ts (lines 1699-1728)
return {
  model: normalizeModelStringForAPI(options.model),
  messages: addCacheBreakpoints(
    messagesForAPI,
    enablePromptCaching,
    options.querySource,
    useCachedMC,
    consumedCacheEdits,
    consumedPinnedEdits,
    options.skipCacheWrite,
  ),
  system,
  tools: allTools,
  tool_choice: options.toolChoice,
  ...(useBetas && { betas: betasParams }),
  metadata: getAPIMetadata(),
  max_tokens: maxOutputTokens,
  thinking,
  ...(temperature !== undefined && { temperature }),
  ...(contextManagement && { context_management: contextManagement }),
  ...(Object.keys(outputConfig).length > 0 && { output_config: outputConfig }),
}
```

---

## 14. Context Window Management

### 14.1 Autocompact Threshold

```typescript
// src/services/compact/autoCompact.ts (lines 32-49)
export function getEffectiveContextWindowSize(model: string): number {
  const reservedTokensForSummary = Math.min(
    getMaxOutputTokensForModel(model),
    MAX_OUTPUT_TOKENS_FOR_SUMMARY,  // 20,000
  )
  let contextWindow = getContextWindowForModel(model, getSdkBetas())

  const autoCompactWindow = process.env.CLAUDE_CODE_AUTO_COMPACT_WINDOW
  if (autoCompactWindow) {
    const parsed = parseInt(autoCompactWindow, 10)
    if (!isNaN(parsed) && parsed > 0) {
      contextWindow = Math.min(contextWindow, parsed)
    }
  }

  return contextWindow - reservedTokensForSummary
}
```

```typescript
// src/services/compact/autoCompact.ts (lines 62-76)
export const AUTOCOMPACT_BUFFER_TOKENS = 13_000

export function getAutoCompactThreshold(model: string): number {
  const effectiveContextWindow = getEffectiveContextWindowSize(model)
  return effectiveContextWindow - AUTOCOMPACT_BUFFER_TOKENS
}
```

Autocompact fires when `tokenCountWithEstimation(messages) > threshold`.

### 14.2 Post-Compact File Restoration

After compaction, recently-read files are restored with tighter budgets:

```typescript
// src/services/compact/compact.ts (lines 122-130)
export const POST_COMPACT_MAX_FILES_TO_RESTORE = 5
export const POST_COMPACT_TOKEN_BUDGET = 50_000
export const POST_COMPACT_MAX_TOKENS_PER_FILE = 5_000
export const POST_COMPACT_MAX_TOKENS_PER_SKILL = 5_000
export const POST_COMPACT_SKILLS_TOKEN_BUDGET = 25_000
```

### 14.3 Read Tool Exemption from Tool Result Budget

The Read tool is explicitly exempt from the disk-persistence tool-result budget:

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 337-342)
export const FileReadTool = buildTool({
  name: FILE_READ_TOOL_NAME,
  searchHint: 'read files, images, PDFs, notebooks',
  // Output is bounded by maxTokens (validateContentTokens). Persisting to a
  // file the model reads back with Read is circular — never persist.
  maxResultSizeChars: Infinity,
  // ...
})
```

This means Read results always stay in-context as full text (never replaced with a file stub), because the system's own token validation is the bound.

---

## 15. Special File Types

### 15.1 Images

Images bypass the 256 KB text cap and use token-budget compression:

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 865-891)
// --- Image (single read, no double-read) ---
if (IMAGE_EXTENSIONS.has(ext)) {
  const data = await readImageWithTokenBudget(resolvedFilePath, maxTokens)
  // ...
  return {
    data,
    ...(metadataText && {
      newMessages: [
        createUserMessage({ content: metadataText, isMeta: true }),
      ],
    }),
  }
}
```

Image serialization for the API:

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 654-668)
case 'image': {
  return {
    tool_use_id: toolUseID,
    type: 'tool_result',
    content: [
      {
        type: 'image',
        source: {
          type: 'base64',
          data: data.file.base64,
          media_type: data.file.type,
        },
      },
    ],
  }
}
```

### 15.2 PDFs

Small PDFs (≤10 pages) are sent as base64 document blocks:

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 999-1016)
return {
  data: pdfData,
  newMessages: [
    createUserMessage({
      content: [
        {
          type: 'document',
          source: {
            type: 'base64',
            media_type: 'application/pdf',
            data: pdfData.file.base64,
          },
        },
      ],
      isMeta: true,
    }),
  ],
}
```

Large PDFs (>10 pages) without the `pages` parameter throw an error:

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 948-954)
if (pageCount !== null && pageCount > PDF_AT_MENTION_INLINE_THRESHOLD) {
  throw new Error(
    `This PDF has ${pageCount} pages, which is too many to read at once. ` +
      `Use the pages parameter to read specific page ranges (e.g., pages: "1-5"). ` +
      `Maximum ${PDF_MAX_PAGES_PER_READ} pages per request.`,
  )
}
```

API limits from `src/constants/apiLimits.ts`:

```typescript
export const PDF_MAX_PAGES_PER_READ = 20
export const PDF_AT_MENTION_INLINE_THRESHOLD = 10
export const PDF_MAX_EXTRACT_SIZE = 100 * 1024 * 1024 // 100 MB
export const API_MAX_MEDIA_PER_REQUEST = 100
```

### 15.3 Jupyter Notebooks

```typescript
// src/tools/FileReadTool/FileReadTool.ts (lines 821-863)
// --- Notebook ---
if (ext === 'ipynb') {
  const cells = await readNotebook(resolvedFilePath)
  const cellsJson = jsonStringify(cells)

  const cellsJsonBytes = Buffer.byteLength(cellsJson)
  if (cellsJsonBytes > maxSizeBytes) {
    throw new Error(
      `Notebook content (${formatFileSize(cellsJsonBytes)}) exceeds maximum allowed size (${formatFileSize(maxSizeBytes)}). ` +
        `Use ${BASH_TOOL_NAME} with jq to read specific portions:\n` +
        `  cat "${file_path}" | jq '.cells[:20]' # First 20 cells\n` +
        `  cat "${file_path}" | jq '.cells[100:120]' # Cells 100-120\n` +
        `  cat "${file_path}" | jq '.cells | length' # Count total cells\n` +
        `  cat "${file_path}" | jq '.cells[] | select(.cell_type=="code") | .source' # All code sources`,
    )
  }

  await validateContentTokens(cellsJson, ext, maxTokens)

  return {
    data: {
      type: 'notebook' as const,
      file: { filePath: file_path, cells },
    },
  }
}
```

---

## 16. Complete Limits Reference

| Limit | Value | Source Location | When Applied | On Exceed |
|-------|-------|-----------------|--------------|-----------|
| **File size (pre-read)** | 256 KB | `src/utils/file.ts:48` | Text read without `limit` param | `FileTooLargeError` thrown |
| **Output tokens** | 25,000 | `src/tools/FileReadTool/limits.ts:18` | All text/notebook reads (post-read) | `MaxFileReadTokenExceededError` thrown |
| **Fast-path threshold** | 10 MB | `src/utils/readFileInRange.ts:44` | Routing decision | Switches to streaming path |
| **@-mention truncation lines** | 2,000 | `src/tools/FileReadTool/prompt.ts:10` | Fallback on size/token error | Truncated, `truncated: true` flag |
| **Directory entries** | 1,000 | `src/utils/attachments.ts:1922` | `@folder` mentions | Truncation marker added |
| **PDF inline threshold** | 10 pages | `src/constants/apiLimits.ts:83` | `@file.pdf` mentions | Becomes `pdf_reference` |
| **PDF pages per Read** | 20 | `src/constants/apiLimits.ts:77` | Read tool `pages` param | Error thrown |
| **PDF max extract size** | 100 MB | `src/constants/apiLimits.ts:72` | PDF page extraction | Rejected |
| **Media items per request** | 100 | `src/constants/apiLimits.ts:94` | All images + PDFs in one call | Stripped with `stripExcessMediaItems()` |
| **IDE selection** | 2,000 chars | `src/utils/messages.ts:3614` | Selected text attachment | Truncated |
| **Post-compact per-file tokens** | 5,000 | `src/services/compact/compact.ts:124` | File restoration after compaction | Truncated |
| **Post-compact total budget** | 50,000 | `src/services/compact/compact.ts:123` | All restored files | Files dropped |
| **Post-compact max files** | 5 | `src/services/compact/compact.ts:122` | File restoration | Excess dropped |
| **Autocompact buffer** | 13,000 tokens | `src/services/compact/autoCompact.ts:62` | Triggers compaction | Conversation summarized |

---

## End-to-End Example: `@src/bigFile.ts fix this bug`

Step-by-step execution:

1. **Input parsing** — `processUserInput()` calls `getAttachmentMessages()`
2. **@-extraction** — `extractAtMentionedFiles("@src/bigFile.ts fix this bug")` → `["src/bigFile.ts"]`
3. **Path resolution** — `expandPath("src/bigFile.ts")` → `/absolute/path/src/bigFile.ts`
4. **Permission check** — `isFileReadDenied()` → passes
5. **Stat + size gate** — `stat()` returns 300 KB → exceeds 256 KB → `return null` (rejected silently for @-mentions)
6. **OR if within size limit (say 200 KB):**
   - `readFileInRange()` reads entire file (fast path, < 10 MB)
   - `validateContentTokens()`: rough estimate = 200000/4 = 50000 > 25000/4=6250 → calls API
   - API returns 28000 tokens > 25000 → throws `MaxFileReadTokenExceededError`
7. **Fallback** — `readTruncatedFile()`: reads first 2000 lines with `FileReadTool.call({ file_path, offset: 1, limit: 2000 })`
   - `readFileInRange()` with `limit=2000`: `maxBytes` NOT passed (limit present), so no byte check
   - `validateContentTokens()` on 2000 lines → passes (under 25K tokens)
8. **Attachment created** — `{ type: 'file', truncated: true, content: { type: 'text', file: { content, numLines: 2000, ... } } }`
9. **Messages built** — `[UserMessage("fix this bug"), AttachmentMessage(file)]`
10. **Query loop starts** — apply budget, snip, microcompact, collapse
11. **prependUserContext** — CLAUDE.md + date as first `<system-reminder>` message
12. **normalizeMessagesForAPI** — attachment becomes:
    - Synthetic: `"Called the Read tool with: {\"file_path\":\"src/bigFile.ts\"}"`
    - Result: `"Result of calling the Read tool:\n     1→import foo...\n     2→..."` (line-numbered)
    - Meta: `"Note: The file was truncated to the first 2000 lines..."`
    - All wrapped in `<system-reminder>` tags
13. **System prompt assembly** — attribution + CLI prefix + main prompt + git status → TextBlockParam[]
14. **API call** — `beta.messages.create({ system, messages, tools, thinking, ... })`
15. **Model responds** — may call `Read` with `offset: 2001, limit: 2000` to see more

---

## Key Architectural Decisions

1. **Throw over truncate** — The default is to throw errors (not silently truncate) because a ~100-byte error message is cheaper than 25K tokens of potentially irrelevant content.

2. **Two-stage token counting** — Fast heuristic first (division by bytes-per-token ratio), API call only when the estimate exceeds 25% of limit. This avoids expensive API calls for most files.

3. **Read is exempt from tool-result budget** — Persisting Read output to disk and making the model re-read it would be circular. The tool's own token cap is the bound.

4. **Deduplication by mtime** — Same file+range at same mtime returns a ~100-byte stub instead of re-sending full content. Saves ~2.64% of fleet cache_creation tokens.

5. **Directories are intentionally shallow** — No recursive reading exists. The model must iteratively explore with Glob/Grep/Read tools. This prevents unbounded context consumption.

6. **Synthetic tool messages** — File attachments are disguised as Read tool calls so the model sees a consistent interface regardless of whether it initiated the read or the user attached it.

---
---

# Chapter 2: Large Output Generation — How the Model Produces Big Code Without Losing Context

This chapter documents how the system enables the LLM to output large amounts of code (multiple files, 1000+ lines) without truncation or context loss. The architecture uses three complementary strategies: **per-response output token budgets with automatic escalation/recovery**, **multi-turn agentic tool-use loops**, and **streaming tool execution with concurrency control**.

---

## Table of Contents (Chapter 2)

1. [Output Token Budget: Configuration & Escalation](#c2-1-output-token-budget)
2. [The Agentic Loop: Multi-Turn Tool Execution](#c2-2-the-agentic-loop)
3. [Streaming: How Blocks Are Accumulated and Tools Start Early](#c2-3-streaming)
4. [Max-Tokens Recovery: Automatic Continuation on Truncation](#c2-4-max-tokens-recovery)
5. [Write & Edit Tools: How Large Code Is Actually Output](#c2-5-write--edit-tools)
6. [Tool Concurrency & Ordering](#c2-6-tool-concurrency--ordering)
7. [Thinking Budget: Extended Reasoning Configuration](#c2-7-thinking-budget)
8. [Token Budget Feature: User-Controlled Turn Length](#c2-8-token-budget-feature)
9. [Why the Model Never "Loses Context" on Large Outputs](#c2-9-why-context-is-preserved)

---

## C2-1. Output Token Budget

### Per-Model Defaults

Each model has a default and upper-limit for output tokens:

```typescript
// src/utils/context.ts (lines 9, 14-25, 149-210)
export const MODEL_CONTEXT_WINDOW_DEFAULT = 200_000

const MAX_OUTPUT_TOKENS_DEFAULT = 32_000
const MAX_OUTPUT_TOKENS_UPPER_LIMIT = 64_000

// Capped default for slot-reservation optimization. BQ p99 output = 4,911
// tokens, so 32k/64k defaults over-reserve 8-16x slot capacity.
export const CAPPED_DEFAULT_MAX_TOKENS = 8_000
export const ESCALATED_MAX_TOKENS = 64_000

export function getModelMaxOutputTokens(model: string): {
  default: number
  upperLimit: number
} {
  const m = getCanonicalName(model)

  if (m.includes('opus-4-6')) {
    defaultTokens = 64_000
    upperLimit = 128_000
  } else if (m.includes('sonnet-4-6')) {
    defaultTokens = 32_000
    upperLimit = 128_000
  } else if (
    m.includes('opus-4-5') ||
    m.includes('sonnet-4') ||
    m.includes('haiku-4')
  ) {
    defaultTokens = 32_000
    upperLimit = 64_000
  } else if (m.includes('opus-4-1') || m.includes('opus-4')) {
    defaultTokens = 32_000
    upperLimit = 32_000
  } else if (m.includes('claude-3-opus')) {
    defaultTokens = 4_096
    upperLimit = 4_096
  } else {
    defaultTokens = MAX_OUTPUT_TOKENS_DEFAULT
    upperLimit = MAX_OUTPUT_TOKENS_UPPER_LIMIT
  }

  return { default: defaultTokens, upperLimit }
}
```

### Resolution Chain (Priority Order)

The effective `max_tokens` sent to the API is resolved in `getMaxOutputTokensForModel`:

```typescript
// src/services/api/claude.ts (lines 3399-3418)
export function getMaxOutputTokensForModel(model: string): number {
  const maxOutputTokens = getModelMaxOutputTokens(model)

  const defaultTokens = isMaxTokensCapEnabled()
    ? Math.min(maxOutputTokens.default, CAPPED_DEFAULT_MAX_TOKENS)
    : maxOutputTokens.default

  const result = validateBoundedIntEnvVar(
    'CLAUDE_CODE_MAX_OUTPUT_TOKENS',
    process.env.CLAUDE_CODE_MAX_OUTPUT_TOKENS,
    defaultTokens,
    maxOutputTokens.upperLimit,
  )
  return result.effective
}
```

**Precedence:**
1. `retryContext.maxTokensOverride` — set by retry logic on context-overflow 400 errors
2. `options.maxOutputTokensOverride` — set by query loop during escalation
3. `CLAUDE_CODE_MAX_OUTPUT_TOKENS` env var — user override
4. `isMaxTokensCapEnabled()` (GrowthBook `tengu_otk_slot_v1`) → cap to 8K
5. Model's default (32K/64K depending on model)

### How It Reaches the API Request

```typescript
// src/services/api/claude.ts (lines 1590-1715, condensed)
function paramsFromContext({ model, thinkingConfig }) {
  const maxOutputTokens =
    retryContext?.maxTokensOverride ||
    options.maxOutputTokensOverride ||
    getMaxOutputTokensForModel(options.model)

  return {
    model: normalizeModelStringForAPI(options.model),
    max_tokens: maxOutputTokens,
    thinking,
    // ...
  }
}
```

The query loop passes the override:

```typescript
// src/query.ts (line 687)
maxOutputTokensOverride,
```

---

## C2-2. The Agentic Loop

The core architectural pattern for producing large output is NOT generating one giant text blob. Instead, the model uses **tool calls** (Write, Edit, Bash) across multiple API round-trips. Each round-trip can output up to `max_tokens` tokens, and the loop continues as long as there are tools to run.

### Loop Structure

```typescript
// src/query.ts (conceptual structure, lines 219+)
export async function* query(params: QueryParams) {
  let state: State = { /* initial state */ }

  while (true) {  // The agentic loop
    // 1. Pre-process messages (compact, snip, collapse)
    // 2. Call the model API (streaming)
    // 3. Collect tool_use blocks from response
    // 4. Execute tools
    // 5. Decide: loop again or stop?

    if (!needsFollowUp) {
      // No tools called → model is done
      return { reason: 'completed' }
    }

    // Tools were called → append results and continue
    state = {
      messages: [...messagesForQuery, ...assistantMessages, ...toolResults],
      turnCount: nextTurnCount,
      transition: { reason: 'next_turn' },
    }
  }
}
```

### Tool Detection — Block Presence, NOT `stop_reason`

The system explicitly does NOT trust `stop_reason === 'tool_use'`:

```typescript
// src/query.ts (lines 553-558)
// Note: stop_reason === 'tool_use' is unreliable -- it's not always set correctly.
// Set during streaming whenever a tool_use block arrives — the sole
// loop-exit signal. If false after streaming, we're done (modulo stop-hook retry).
const toolUseBlocks: ToolUseBlock[] = []
let needsFollowUp = false
```

Detection happens during streaming:

```typescript
// src/query.ts (lines 826-834)
if (message.type === 'assistant') {
  assistantMessages.push(message)

  const msgToolUseBlocks = message.message.content.filter(
    content => content.type === 'tool_use',
  ) as ToolUseBlock[]
  if (msgToolUseBlocks.length > 0) {
    toolUseBlocks.push(...msgToolUseBlocks)
    needsFollowUp = true
  }
}
```

### Turn Limit

```typescript
// src/query.ts (lines 1704-1711)
if (maxTurns && nextTurnCount > maxTurns) {
  yield createAttachmentMessage({
    type: 'max_turns_reached',
    maxTurns,
    turnCount: nextTurnCount,
  })
  return { reason: 'max_turns', turnCount: nextTurnCount }
}
```

### Loop Continuation

After all tools complete, the loop recurses with the full accumulated history:

```typescript
// src/query.ts (lines 1714-1728)
const next: State = {
  messages: [...messagesForQuery, ...assistantMessages, ...toolResults],
  toolUseContext: toolUseContextWithQueryTracking,
  autoCompactTracking: tracking,
  turnCount: nextTurnCount,
  maxOutputTokensRecoveryCount: 0,
  hasAttemptedReactiveCompact: false,
  pendingToolUseSummary: nextPendingToolUseSummary,
  maxOutputTokensOverride: undefined,
  stopHookActive,
  transition: { reason: 'next_turn' },
}
state = next
```

Each iteration is a **new API call** with the entire conversation history. The model sees all previous tool calls and results, maintaining full context.

---

## C2-3. Streaming

### Raw SSE Stream (Not BetaMessageStream)

The system uses the raw SDK stream to avoid O(n^2) partial JSON parsing that `BetaMessageStream` performs on tool inputs:

```typescript
// src/services/api/claude.ts (lines 1818-1836)
// Use raw stream instead of BetaMessageStream to avoid O(n²) partial JSON parsing
// BetaMessageStream calls partialParse() on every input_json_delta, which we don't need
// since we handle tool input accumulation ourselves
const result = await anthropic.beta.messages
  .create(
    { ...params, stream: true },
    { signal, ...(clientRequestId && { headers: { ... } }) },
  )
  .withResponse()
```

### Block Accumulation Per Event Type

| Event | Action |
|-------|--------|
| `message_start` | Initialize `partialMessage`, record TTFT |
| `content_block_start` | Init block: `text → ''`, `tool_use → input: ''`, `thinking → thinking: ''` |
| `content_block_delta` | Append: `text_delta`, `input_json_delta`, `thinking_delta`, `signature_delta` |
| `content_block_stop` | Finalize block, **yield one `AssistantMessage` per block** |
| `message_delta` | Write final `usage` + `stop_reason` onto last yielded message |

Tool input JSON accumulation (large code payloads):

```typescript
// src/services/api/claude.ts (lines 2087-2111)
case 'input_json_delta':
  contentBlock.input += delta.partial_json
  break
```

### One AssistantMessage Per Block

At `content_block_stop`, a message is yielded containing **only that block**:

```typescript
// src/services/api/claude.ts (lines 2192-2210)
const m: AssistantMessage = {
  message: {
    ...partialMessage,
    content: normalizeContentFromAPI(
      [contentBlock] as BetaContentBlock[],
      tools,
      options.agentId,
    ),
  },
  type: 'assistant',
  uuid: randomUUID(),
  timestamp: new Date().toISOString(),
}
newMessages.push(m)
yield m
```

This means a response with `[text, tool_use(Write), tool_use(Edit)]` yields 3 separate messages during streaming, allowing tools to start before the full response completes.

### Streaming Tool Execution

Tools start as their blocks complete, concurrently with continued streaming:

```typescript
// src/query.ts (lines 837-844)
if (
  streamingToolExecutor &&
  !toolUseContext.abortController.signal.aborted
) {
  for (const toolBlock of msgToolUseBlocks) {
    streamingToolExecutor.addTool(toolBlock, message)
  }
}
```

Completed results are yielded mid-stream:

```typescript
// src/query.ts (lines 847-862)
if (streamingToolExecutor && !toolUseContext.abortController.signal.aborted) {
  for (const result of streamingToolExecutor.getCompletedResults()) {
    if (result.message) {
      yield result.message
      toolResults.push(
        ...normalizeMessagesForAPI(
          [result.message],
          toolUseContext.options.tools,
        ).filter(_ => _.type === 'user'),
      )
    }
  }
}
```

After stream ends, remaining tools drain:

```typescript
// src/query.ts (lines 1380-1408)
const toolUpdates = streamingToolExecutor
  ? streamingToolExecutor.getRemainingResults()
  : runTools(toolUseBlocks, assistantMessages, canUseTool, toolUseContext)

for await (const update of toolUpdates) {
  if (update.message) {
    yield update.message
    toolResults.push(
      ...normalizeMessagesForAPI(
        [update.message],
        toolUseContext.options.tools,
      ).filter(_ => _.type === 'user'),
    )
  }
}
```

---

## C2-4. Max-Tokens Recovery

When the model hits its output limit mid-generation, the system automatically recovers.

### Detection in the Stream

```typescript
// src/services/api/claude.ts (lines 2266-2291)
if (stopReason === 'max_tokens') {
  logEvent('tengu_max_tokens_reached', { max_tokens: maxOutputTokens })
  yield createAssistantAPIErrorMessage({
    content: `${API_ERROR_MESSAGE_PREFIX}: Claude's response exceeded the ${
      maxOutputTokens
    } output token maximum. To configure this behavior, set the CLAUDE_CODE_MAX_OUTPUT_TOKENS environment variable.`,
    apiError: 'max_output_tokens',
    error: 'max_output_tokens',
  })
}

if (stopReason === 'model_context_window_exceeded') {
  // Reuse the max_output_tokens recovery path — from the model's
  // perspective, both mean "response was cut off, continue from
  // where you left off."
  yield createAssistantAPIErrorMessage({
    content: `${API_ERROR_MESSAGE_PREFIX}: The model has reached its context window limit.`,
    apiError: 'max_output_tokens',
    error: 'max_output_tokens',
  })
}
```

### Withholding From Consumers

The error is NOT immediately surfaced to the user/SDK — it's withheld until recovery is attempted:

```typescript
// src/query.ts (lines 164, 175-179)
const MAX_OUTPUT_TOKENS_RECOVERY_LIMIT = 3

function isWithheldMaxOutputTokens(
  msg: Message | StreamEvent | undefined,
): msg is AssistantMessage {
  return msg?.type === 'assistant' && msg.apiError === 'max_output_tokens'
}
```

```typescript
// src/query.ts (lines 820-825)
if (isWithheldMaxOutputTokens(message)) {
  withheld = true
}
if (!withheld) {
  yield yieldMessage
}
```

### Recovery Tier 1: Escalate 8K → 64K (Same Request)

When the slot-reservation cap (8K) was hit, retry at 64K without adding messages:

```typescript
// src/query.ts (lines 1188-1221)
if (isWithheldMaxOutputTokens(lastMessage)) {
  const capEnabled = getFeatureValue_CACHED_MAY_BE_STALE(
    'tengu_otk_slot_v1',
    false,
  )
  if (
    capEnabled &&
    maxOutputTokensOverride === undefined &&
    !process.env.CLAUDE_CODE_MAX_OUTPUT_TOKENS
  ) {
    logEvent('tengu_max_tokens_escalate', {
      escalatedTo: ESCALATED_MAX_TOKENS,
    })
    const next: State = {
      messages: messagesForQuery,
      maxOutputTokensOverride: ESCALATED_MAX_TOKENS,
      transition: { reason: 'max_output_tokens_escalate' },
      // ... rest of state
    }
    state = next
    continue  // Retry same request with higher limit
  }
```

### Recovery Tier 2: Multi-Turn Continuation (Up to 3 Attempts)

If still truncated, inject a meta user message and loop again with the truncated output preserved in history:

```typescript
// src/query.ts (lines 1223-1252)
if (maxOutputTokensRecoveryCount < MAX_OUTPUT_TOKENS_RECOVERY_LIMIT) {
  const recoveryMessage = createUserMessage({
    content:
      `Output token limit hit. Resume directly — no apology, no recap of what you were doing. ` +
      `Pick up mid-thought if that is where the cut happened. Break remaining work into smaller pieces.`,
    isMeta: true,
  })

  const next: State = {
    messages: [
      ...messagesForQuery,
      ...assistantMessages,  // Truncated output is preserved
      recoveryMessage,       // Nudge to continue
    ],
    maxOutputTokensRecoveryCount: maxOutputTokensRecoveryCount + 1,
    maxOutputTokensOverride: undefined,
    transition: {
      reason: 'max_output_tokens_recovery',
      attempt: maxOutputTokensRecoveryCount + 1,
    },
    // ... rest of state
  }
  state = next
  continue
}

// Recovery exhausted — surface the withheld error now.
yield lastMessage
```

### Recovery Flow Diagram

```
Model response → stop_reason: max_tokens
                         │
                         ▼
              ┌─────────────────────┐
              │ Was 8K cap active?   │
              └──────────┬──────────┘
                    yes  │  no
                         │
              ┌──────────▼──────────┐
              │ Retry at 64K        │─────────── success → done
              │ (same request)      │
              └──────────┬──────────┘
                    still truncated
                         │
              ┌──────────▼──────────┐
              │ Inject recovery msg │
              │ "Resume directly…"  │──── attempt 1/3
              └──────────┬──────────┘
                    still truncated
                         │
              ┌──────────▼──────────┐
              │ Inject recovery msg │──── attempt 2/3
              └──────────┬──────────┘
                    still truncated
                         │
              ┌──────────▼──────────┐
              │ Inject recovery msg │──── attempt 3/3
              └──────────┬──────────┘
                    still truncated
                         │
              ┌──────────▼──────────┐
              │ Surface error to     │
              │ user/SDK             │
              └─────────────────────┘
```

---

## C2-5. Write & Edit Tools: How Large Code Is Actually Output

The model does not produce raw text output for code generation. Instead it calls **tools** that write to disk. This is the key architectural insight: large code output is broken into discrete tool calls, each of which is one content block in the response.

### The Write Tool (`Write`)

**Location:** `src/tools/FileWriteTool/FileWriteTool.ts`

```typescript
// src/tools/FileWriteTool/FileWriteTool.ts (lines 56-64)
const inputSchema = lazySchema(() =>
  z.strictObject({
    file_path: z.string().describe('The absolute path to the file to write'),
    content: z.string().describe('The content to write to the file'),
  }),
)
```

**No max length on `content`** — the model can write any amount of code in a single tool call. The only practical limit is the per-response `max_tokens` budget which constrains how much JSON the model can emit for the `tool_use` input.

**Tool result is tiny** — keeps context lean:

```typescript
// src/tools/FileWriteTool/FileWriteTool.ts (lines 418-432)
mapToolResultToToolResultBlockParam({ filePath, type }, toolUseID) {
  switch (type) {
    case 'create':
      return {
        tool_use_id: toolUseID,
        type: 'tool_result',
        content: `File created successfully at: ${filePath}`,
      }
    case 'update':
      return {
        tool_use_id: toolUseID,
        type: 'tool_result',
        content: `The file ${filePath} has been updated successfully.`,
      }
  }
},
```

### The Edit Tool (`Edit`)

**Location:** `src/tools/FileEditTool/FileEditTool.ts`

```typescript
// src/tools/FileEditTool/types.ts (lines 6-18)
const inputSchema = lazySchema(() =>
  z.strictObject({
    file_path: z.string().describe('The absolute path to the file to modify'),
    old_string: z.string().describe('The text to replace'),
    new_string: z.string().describe('The text to replace it with'),
    replace_all: semanticBoolean(
      z.boolean().default(false).optional(),
    ).describe('Replace all occurrences of old_string (default false)'),
  }),
)
```

File size limit for Edit targets (not the output content size):

```typescript
// src/tools/FileEditTool/FileEditTool.ts (lines 79-84)
// V8/Bun string length limit is ~2^30 characters (~1 billion).
const MAX_EDIT_FILE_SIZE = 1024 * 1024 * 1024 // 1 GiB (stat bytes)
```

### Where Large Code Lives in Context

The large code payload sits in the **assistant message's `tool_use` block input**:

```
Assistant message: {
  content: [
    { type: "text", text: "I'll create the file..." },
    { type: "tool_use", id: "xyz", name: "Write", input: {
        file_path: "/path/to/file.ts",
        content: "... 500 lines of code ..."  ← THE LARGE PAYLOAD
    }}
  ]
}
```

The tool **result** back to the model is ~50 characters ("File created successfully at: /path/to/file.ts"), not the file content. This asymmetry is critical: the model emits large content, but only receives tiny confirmations back. This keeps the growing context lean for subsequent turns.

### Practical Output Limits Per Tool Call

There is NO explicit limit on `content` / `old_string` / `new_string` length in the tool schemas. The effective bound is:

- **`max_tokens` for the whole response** — the JSON representation of the `tool_use` block (including the code content) counts against this.
- For a 64K token response with thinking disabled, a single Write tool call can contain roughly **~50K tokens of code** (after accounting for JSON overhead and any text blocks).
- For Opus 4.6 with 128K upper limit, even more.

### Multiple Files in One Response

The model can emit multiple `tool_use` blocks in a single API response:

```
Response: [
  { type: "tool_use", name: "Write", input: { file_path: "a.ts", content: "..." } },
  { type: "tool_use", name: "Write", input: { file_path: "b.ts", content: "..." } },
  { type: "tool_use", name: "Edit", input: { file_path: "c.ts", old_string: "...", new_string: "..." } },
]
```

These execute serially (Write/Edit are not concurrency-safe), then the loop continues with all results appended. The model can then output MORE tool calls on the next turn, referencing what it already wrote.

---

## C2-6. Tool Concurrency & Ordering

### The StreamingToolExecutor

```typescript
// src/services/tools/StreamingToolExecutor.ts (lines 34-39)
/**
 * Executes tools as they stream in with concurrency control.
 * - Concurrent-safe tools can execute in parallel with other concurrent-safe tools
 * - Non-concurrent tools must execute alone (exclusive access)
 * - Results are buffered and emitted in the order tools were received
 */
export class StreamingToolExecutor {
```

### Concurrency Check

```typescript
// src/services/tools/StreamingToolExecutor.ts (lines 129-135)
private canExecuteTool(isConcurrencySafe: boolean): boolean {
  const executingTools = this.tools.filter(t => t.status === 'executing')
  return (
    executingTools.length === 0 ||
    (isConcurrencySafe && executingTools.every(t => t.isConcurrencySafe))
  )
}
```

### Tool Classification

- **Concurrent-safe** (can run in parallel): `Read`, `Glob`, `Grep` — read-only tools
- **Non-concurrent** (must run alone, in order): `Write`, `Edit`, `Bash` — side-effecting tools

This means `Write(fileA), Write(fileB), Edit(fileC)` in one response → three **serial** executions in order.

### Batch Partitioning (Non-Streaming Fallback)

```typescript
// src/services/tools/toolOrchestration.ts (lines 86-115)
function partitionToolCalls(toolUseMessages, toolUseContext): Batch[] {
  return toolUseMessages.reduce((acc: Batch[], toolUse) => {
    const tool = findToolByName(toolUseContext.options.tools, toolUse.name)
    const isConcurrencySafe = tool?.isConcurrencySafe(toolUse.input) ?? false

    if (isConcurrencySafe && acc[acc.length - 1]?.isConcurrencySafe) {
      acc[acc.length - 1]!.blocks.push(toolUse)
    } else {
      acc.push({ isConcurrencySafe, blocks: [toolUse] })
    }
    return acc
  }, [])
}
```

### `readFileState` Chaining

After each Write/Edit, `readFileState` is updated with the new content:

```typescript
// src/tools/FileWriteTool/FileWriteTool.ts (lines ~320-330)
readFileState.set(fullFilePath, {
  content,
  timestamp: getFileModificationTime(fullFilePath),
  offset: undefined,
  limit: undefined,
})
```

This means the NEXT tool call in the same turn can immediately edit the file that was just written — no extra Read needed. The state chains through sequential tool execution within a turn.

---

## C2-7. Thinking Budget: Extended Reasoning Configuration

### Configuration Type

```typescript
// src/utils/thinking.ts (lines 10-13)
export type ThinkingConfig =
  | { type: 'adaptive' }
  | { type: 'enabled'; budgetTokens: number }
  | { type: 'disabled' }
```

### API Mapping

```typescript
// src/services/api/claude.ts (lines 1596-1630)
const hasThinking =
  thinkingConfig.type !== 'disabled' &&
  !isEnvTruthy(process.env.CLAUDE_CODE_DISABLE_THINKING)

if (hasThinking && modelSupportsThinking(options.model)) {
  if (
    !isEnvTruthy(process.env.CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING) &&
    modelSupportsAdaptiveThinking(options.model)
  ) {
    // Opus 4.6 / Sonnet 4.6: adaptive thinking (no explicit budget)
    thinking = { type: 'adaptive' }
  } else {
    // Older models: explicit budget mode
    let thinkingBudget = getMaxThinkingTokensForModel(options.model)
    if (
      thinkingConfig.type === 'enabled' &&
      thinkingConfig.budgetTokens !== undefined
    ) {
      thinkingBudget = thinkingConfig.budgetTokens
    }
    // API constraint: budget_tokens must be strictly less than max_tokens
    thinkingBudget = Math.min(maxOutputTokens - 1, thinkingBudget)
    thinking = {
      budget_tokens: thinkingBudget,
      type: 'enabled',
    }
  }
}
```

### Default Budget

```typescript
// src/utils/context.ts (lines 219-221)
export function getMaxThinkingTokensForModel(model: string): number {
  return getModelMaxOutputTokens(model).upperLimit - 1
}
```

So for Sonnet 4.6 with upperLimit 128K: thinking budget = 127,999 tokens. But adaptive mode (which it actually uses) has no explicit budget — the API decides how much to think.

### Key Constraints

- **`budget_tokens < max_tokens`** — API enforced. The code ensures this with `Math.min(maxOutputTokens - 1, thinkingBudget)`.
- **Thinking tokens do NOT count toward `max_tokens`** in adaptive mode — the model can think extensively and still output full tool calls.
- **Thinking blocks are preserved** through tool trajectories (thinking → tool_use → tool_result → next assistant must keep the thinking block in context).

### Non-Streaming Fallback Adjustment

```typescript
// src/services/api/claude.ts (lines 3350-3391)
export const MAX_NON_STREAMING_TOKENS = 64_000

export function adjustParamsForNonStreaming<T>(params: T, maxTokensCap: number): T {
  const cappedMaxTokens = Math.min(params.max_tokens, maxTokensCap)
  // Adjust thinking budget if it would exceed capped max_tokens
  return { ...adjustedParams, max_tokens: cappedMaxTokens }
}
```

---

## C2-8. Token Budget Feature: User-Controlled Turn Length

Users can request extended output via the `+500k` syntax (e.g. `+500k refactor the entire module`). This is a client-side turn-continuation mechanism distinct from `max_tokens`:

### How It Works

After the model completes a turn (no tools), the system checks if the token budget has been met:

```typescript
// src/query.ts (lines 1308-1339)
if (feature('TOKEN_BUDGET')) {
  const decision = checkTokenBudget(
    budgetTracker!,
    toolUseContext.agentId,
    getCurrentTurnTokenBudget(),
    getTurnOutputTokens(),
  )

  if (decision.action === 'continue') {
    incrementBudgetContinuationCount()
    state = {
      messages: [
        ...messagesForQuery,
        ...assistantMessages,
        createUserMessage({
          content: decision.nudgeMessage,
          isMeta: true,
        }),
      ],
      transition: { reason: 'token_budget_continuation' },
    }
    continue  // Force another model turn
  }
}
```

### The Nudge Message

```typescript
// src/utils/tokenBudget.ts (lines 66-72)
export function getBudgetContinuationMessage(
  pct: number,
  turnTokens: number,
  budget: number,
): string {
  return `Stopped at ${pct}% of token target (${fmt(turnTokens)} / ${fmt(budget)}). Keep working — do not summarize.`
}
```

### Stop Conditions

- Continue while under 90% of budget
- Stop early on "diminishing returns" (< 500 new tokens after 3+ continuations)

---

## C2-9. Why the Model Never "Loses Context" on Large Outputs

The system ensures context preservation through several mechanisms:

### 1. Full History Accumulates

Every loop iteration sends the **complete** conversation history (after compaction) to the API:

```typescript
// src/query.ts (line 660)
messages: prependUserContext(messagesForQuery, userContext),
```

Where `messagesForQuery` includes ALL previous assistant messages and tool results.

### 2. Tool Results Are Tiny, Tool Inputs Are Large

The asymmetry is critical:
- **Model output** (tool_use inputs): can be 50K+ tokens of code
- **Tool results** back to model: ~50-100 characters ("File created successfully")

This means context grows slowly even when the model outputs thousands of lines. A turn that writes 5 files adds ~250 characters of tool results, not 5x file contents.

### 3. Compaction Preserves Recent Context

When context gets large, autocompact summarizes OLD history but preserves recent messages:

```typescript
// src/services/compact/compact.ts (lines 122-124)
export const POST_COMPACT_MAX_FILES_TO_RESTORE = 5
export const POST_COMPACT_TOKEN_BUDGET = 50_000
export const POST_COMPACT_MAX_TOKENS_PER_FILE = 5_000
```

After compaction, the 5 most recently read files are restored into context so the model doesn't lose track of what it was working on.

### 4. Recovery Preserves Truncated Output

When `max_tokens` is hit, the truncated assistant message is kept in history:

```typescript
// src/query.ts (line 1232-1236)
messages: [
  ...messagesForQuery,
  ...assistantMessages,  // Truncated output preserved
  recoveryMessage,       // "Resume directly..."
],
```

The model sees what it already wrote and continues from where it left off.

### 5. `readFileState` Tracks What's Written

Every Write/Edit updates a state map with the full file content. This enables:
- Deduplication (don't re-send unchanged files)
- Staleness detection (don't overwrite external edits)
- State chaining (next tool sees previous tool's output)

### 6. The Model Chooses Tool Granularity

The system imposes no artificial limits on how the model structures its work. The model can:
- Write one large file in a single Write call (limited by `max_tokens`)
- Split a large file into multiple Edit calls across turns
- Write multiple files in one response (multiple tool_use blocks)
- Read files back to verify before continuing

The prompt explicitly guides toward Edit for modifications:

```typescript
// src/tools/FileWriteTool/prompt.ts (lines 10-17)
`Prefer the Edit tool for modifying existing files — it only sends the diff. Only use this tool to create new files or for complete rewrites.`
```

---

## Summary: The Complete Output Architecture

| Layer | Mechanism | Purpose |
|-------|-----------|---------|
| **Per-response** | `max_tokens` (8K→64K→128K) | Cap single API call output |
| **Auto-escalation** | 8K → 64K retry | Handle slot-cap hits transparently |
| **Multi-turn recovery** | 3x continuation nudges | Resume truncated output |
| **Agentic loop** | `while (true)` + tool detection | Unlimited turns of tool calls |
| **Streaming execution** | Tools start during stream | Overlap I/O with generation |
| **Tiny tool results** | "File created successfully" | Keep context lean |
| **State chaining** | `readFileState` | Tools build on each other |
| **Context management** | Autocompact + file restore | Prevent overflow while keeping recent files |
| **Token budget** | `+500k` continuation | User-controlled extended generation |
| **Thinking** | Adaptive / budget mode | Extended reasoning without consuming output budget |

The net effect: the model can produce arbitrarily large codebases by using tool calls across an unbounded number of turns, each turn outputting up to 128K tokens, with automatic recovery from truncation and lean context growth.
