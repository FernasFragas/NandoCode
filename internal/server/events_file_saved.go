package server

func emitFileSaved(sess *Session, path string, size int, hash, source string) {
	sess.Emit("file_saved", map[string]any{
		"path": path, "size": size, "sha256": hash, "decision_source": source,
	})
}
