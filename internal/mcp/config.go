package mcp

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Transport describes an MCP server transport.
type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
)

// ServerConfig is one MCP server definition from config.
type ServerConfig struct {
	Name      string
	Enabled   bool
	Trusted   bool
	Transport Transport
	Command   string
	Args      []string
	Env       map[string]string
	URL       string
	Auth      string
}

// Config contains MCP server definitions.
type Config struct {
	Servers []ServerConfig
}

// LoadConfig reads MCP server config from nandocodego config.toml paths.
// User config has priority over project config for duplicate server names.
func LoadConfig(userConfigPath, projectConfigPath string) (Config, []string) {
	user, userWarn := loadConfigFile(userConfigPath, true)
	project, projectWarn := loadConfigFile(projectConfigPath, false)

	byName := make(map[string]ServerConfig)
	for _, s := range project.Servers {
		byName[s.Name] = s
	}
	for _, s := range user.Servers {
		byName[s.Name] = s
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := Config{Servers: make([]ServerConfig, 0, len(byName))}
	for _, name := range names {
		out.Servers = append(out.Servers, byName[name])
	}
	return out, append(userWarn, projectWarn...)
}

func loadConfigFile(path string, trustedDefault bool) (Config, []string) {
	if strings.TrimSpace(path) == "" {
		return Config{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, []string{fmt.Sprintf("%s: %v", path, err)}
	}
	return parseTOML(string(b), path, trustedDefault)
}

func parseTOML(data, source string, trustedDefault bool) (Config, []string) {
	var cfg Config
	var warnings []string
	servers := map[string]*ServerConfig{}
	seenSections := map[string]bool{}
	var sectionName string

	sc := bufio.NewScanner(strings.NewReader(data))
	for lineNo := 1; sc.Scan(); lineNo++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			const prefix = "mcp.servers."
			if strings.HasPrefix(raw, prefix) {
				sectionName = strings.TrimSpace(strings.TrimPrefix(raw, prefix))
				sectionName = sanitizeNameNoFallback(sectionName)
				if sectionName == "" {
					warnings = append(warnings, fmt.Sprintf("%s:%d invalid server section", source, lineNo))
					sectionName = ""
					continue
				}
				if seenSections[sectionName] {
					warnings = append(warnings, fmt.Sprintf("%s:%d duplicate server section %q", source, lineNo, sectionName))
				}
				seenSections[sectionName] = true
				if _, ok := servers[sectionName]; !ok {
					servers[sectionName] = &ServerConfig{
						Name:      sectionName,
						Enabled:   true,
						Trusted:   trustedDefault,
						Transport: TransportStdio,
					}
				}
			} else {
				sectionName = ""
			}
			continue
		}
		if sectionName == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s:%d invalid assignment", source, lineNo))
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		srv := servers[sectionName]
		switch key {
		case "enabled":
			v, err := strconv.ParseBool(value)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s:%d invalid enabled value", source, lineNo))
				continue
			}
			srv.Enabled = v
		case "transport":
			str, err := parseQuoted(value)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s:%d invalid transport value", source, lineNo))
				continue
			}
			switch Transport(str) {
			case TransportStdio, TransportHTTP:
				srv.Transport = Transport(str)
			default:
				warnings = append(warnings, fmt.Sprintf("%s:%d unsupported transport %q", source, lineNo, str))
			}
		case "command":
			str, err := parseQuoted(value)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s:%d invalid command value", source, lineNo))
				continue
			}
			if strings.TrimSpace(str) == "" {
				warnings = append(warnings, fmt.Sprintf("%s:%d empty command", source, lineNo))
				continue
			}
			srv.Command = str
		case "args":
			args, err := parseStringArray(value)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s:%d invalid args: %v", source, lineNo, err))
				continue
			}
			srv.Args = args
		case "env":
			env, err := parseStringMap(value)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s:%d invalid env: %v", source, lineNo, err))
				continue
			}
			srv.Env = env
		case "url":
			str, err := parseQuoted(value)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s:%d invalid url value", source, lineNo))
				continue
			}
			srv.URL = str
		case "auth":
			str, err := parseQuoted(value)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s:%d invalid auth value", source, lineNo))
				continue
			}
			srv.Auth = strings.ToLower(strings.TrimSpace(str))
		case "trusted":
			v, err := strconv.ParseBool(value)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s:%d invalid trusted value", source, lineNo))
				continue
			}
			srv.Trusted = v
		}
	}
	if err := sc.Err(); err != nil {
		warnings = append(warnings, fmt.Sprintf("%s: %v", source, err))
	}

	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s := servers[name]
		if s.Transport == TransportStdio && strings.TrimSpace(s.Command) == "" {
			warnings = append(warnings, fmt.Sprintf("%s: server %q missing command", source, s.Name))
		}
		if s.Transport == TransportHTTP && strings.TrimSpace(s.URL) == "" {
			warnings = append(warnings, fmt.Sprintf("%s: server %q missing url", source, s.Name))
		}
		cfg.Servers = append(cfg.Servers, *s)
	}
	return cfg, warnings
}

func parseQuoted(v string) (string, error) {
	v = strings.TrimSpace(v)
	if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
		return "", fmt.Errorf("not a quoted string")
	}
	u, err := strconv.Unquote(v)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(u), nil
}

func parseStringArray(v string) ([]string, error) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return nil, fmt.Errorf("not an array")
	}
	body := strings.TrimSpace(v[1 : len(v)-1])
	if body == "" {
		return nil, nil
	}
	parts := strings.Split(body, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s, err := parseQuoted(strings.TrimSpace(p))
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func parseStringMap(v string) (map[string]string, error) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "{") || !strings.HasSuffix(v, "}") {
		return nil, fmt.Errorf("not an inline table")
	}
	body := strings.TrimSpace(v[1 : len(v)-1])
	if body == "" {
		return map[string]string{}, nil
	}
	parts := strings.Split(body, ",")
	out := make(map[string]string, len(parts))
	for _, p := range parts {
		key, value, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("invalid key/value %q", strings.TrimSpace(p))
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("empty key")
		}
		str, err := parseQuoted(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid value for %q: %w", key, err)
		}
		out[key] = str
	}
	return out, nil
}
