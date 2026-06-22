package logging

import "log/slog"

// SafeStringAttr builds a string attribute with value redaction applied.
func SafeStringAttr(key, value string) slog.Attr {
	return slog.String(key, Redact(value))
}

// SafeErrorAttr builds an error attribute with redaction applied to the message.
func SafeErrorAttr(err error) slog.Attr {
	if err == nil {
		return slog.String("error", "")
	}
	return slog.String("error", Redact(err.Error()))
}
