package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Environment variable names that override the matching config.json field.
// Precedence is environment > config.json > built-in defaults. A variable that
// is unset or empty leaves the file value in place, so a deployment can pin a
// few settings without restating the whole configuration.
const (
	// EnvConfigJSON supplies the whole configuration as JSON when the file
	// named by -config does not exist. It is decoded exactly like a file, and
	// the individual overrides below still win over it.
	EnvConfigJSON = "CONFIG_JSON"

	envListen            = "OPENCODE2API_LISTEN"
	envListenLegacy      = "LISTEN_ADDRESS"
	envPort              = "PORT"
	envServerKeys        = "OPENCODE2API_SERVER_KEYS"
	envZenKeys           = "OPENCODE2API_ZEN_KEYS"
	envGoKeys            = "OPENCODE2API_GO_KEYS"
	envAnonymous         = "OPENCODE2API_ANONYMOUS"
	envProxies           = "OPENCODE2API_PROXIES"
	envProxyFile         = "OPENCODE2API_PROXYFILE"
	envPrefer            = "OPENCODE2API_PREFER"
	envUpstreamZen       = "OPENCODE2API_UPSTREAM_ZEN"
	envUpstreamGo        = "OPENCODE2API_UPSTREAM_GO"
	envUpstreamAnonymous = "OPENCODE2API_UPSTREAM_ANONYMOUS"
	envRetryMaxAttempts  = "OPENCODE2API_RETRY_MAX_ATTEMPTS"
	envRetryTimeout      = "OPENCODE2API_RETRY_TIMEOUT_SECONDS"
	envModelsRefresh     = "OPENCODE2API_MODELS_REFRESH_SECONDS"
	envFreeOnly          = "OPENCODE2API_FREE_ONLY"
	envLogLevel          = "OPENCODE2API_LOG_LEVEL"
	envLogRingSize       = "OPENCODE2API_LOG_RING_SIZE"
	envWebUIEnabled      = "OPENCODE2API_WEBUI_ENABLED"
	envWebUIListen       = "OPENCODE2API_WEBUI_LISTEN"
	envWebUIListenLegacy = "WEBUI_LISTEN_ADDRESS"
	envWebUIUsername     = "OPENCODE2API_WEBUI_USERNAME"
	envWebUIPassword     = "OPENCODE2API_WEBUI_PASSWORD"
	envWebUISessionTTL   = "OPENCODE2API_WEBUI_SESSION_TTL_MINUTES"
	envWarpEnabled       = "OPENCODE2API_WARP_ENABLED"
	envWarpPoolSize      = "OPENCODE2API_WARP_POOL_SIZE"
	envWarpStack         = "OPENCODE2API_WARP_STACK"
	envWarpSourceIPv6    = "OPENCODE2API_WARP_SOURCE_IPV6"
	envReasoningEffort   = "OPENCODE2API_REASONING_EFFORT"
)

// environmentConfigures reports whether the process environment can stand in
// for a config file. Server keys are the one field Normalize cannot supply, so
// they decide whether an absent file is a configuration or an error.
func environmentConfigures() bool {
	if strings.TrimSpace(os.Getenv(EnvConfigJSON)) != "" {
		return true
	}
	return len(envList(envServerKeys)) > 0
}

// applyEnvironment overlays the OPENCODE2API_* variables on cfg. It runs
// before Normalize, so every value it writes is validated the same way a value
// read from the file is.
func applyEnvironment(cfg *Config) error {
	if value, ok := envString(envListen, envListenLegacy); ok {
		cfg.Listen = value
	} else if port, ok := envString(envPort); ok {
		// PaaS platforms inject the port to listen on and expect the process to
		// bind it. Nothing else reads PORT, so map it to the public interface.
		cfg.Listen = "0.0.0.0:" + port
	}
	if value, ok := envString(envWebUIListen, envWebUIListenLegacy); ok {
		cfg.WebUI.Listen = value
	}
	if values := envList(envServerKeys); len(values) > 0 {
		cfg.ServerKeys = values
	}
	if values := envList(envZenKeys); len(values) > 0 {
		cfg.ZenKeys = values
	}
	if values := envList(envGoKeys); len(values) > 0 {
		cfg.GoKeys = values
	}
	if values := envList(envProxies); len(values) > 0 {
		cfg.Proxies = values
	}
	if err := envBoolInto(envAnonymous, &cfg.Anonymous); err != nil {
		return err
	}
	if value, ok := envString(envProxyFile); ok {
		cfg.ProxyFile = value
	}
	if value, ok := envString(envPrefer); ok {
		cfg.Prefer = Tier(strings.ToLower(value))
	}
	if value, ok := envString(envUpstreamZen); ok {
		cfg.Upstream.Zen = value
	}
	if value, ok := envString(envUpstreamGo); ok {
		cfg.Upstream.Go = value
	}
	if value, ok := envString(envUpstreamAnonymous); ok {
		cfg.Upstream.Anonymous = value
	}
	if err := envIntInto(envRetryMaxAttempts, &cfg.Retry.MaxAttempts); err != nil {
		return err
	}
	if err := envIntInto(envRetryTimeout, &cfg.Retry.TimeoutSeconds); err != nil {
		return err
	}
	if err := envIntInto(envModelsRefresh, &cfg.Models.RefreshSeconds); err != nil {
		return err
	}
	if err := envBoolInto(envFreeOnly, &cfg.Models.FreeOnly); err != nil {
		return err
	}
	if value, ok := envString(envLogLevel); ok {
		cfg.Logging.Level = strings.ToLower(value)
	}
	if err := envIntInto(envLogRingSize, &cfg.Logging.RingSize); err != nil {
		return err
	}
	if err := envBoolInto(envWebUIEnabled, &cfg.WebUI.Enabled); err != nil {
		return err
	}
	if value, ok := envString(envWebUIUsername); ok {
		cfg.WebUI.Username = value
	}
	if value, ok := envString(envWebUIPassword); ok {
		applyEnvPassword(cfg, value)
	}
	if err := envIntInto(envWebUISessionTTL, &cfg.WebUI.SessionTTLMinutes); err != nil {
		return err
	}
	if err := envBoolInto(envWarpEnabled, &cfg.Warp.Enabled); err != nil {
		return err
	}
	if err := envIntInto(envWarpPoolSize, &cfg.Warp.PoolSize); err != nil {
		return err
	}
	if value, ok := envString(envWarpStack); ok {
		cfg.Warp.Stack = value
	}
	if value, ok := envString(envWarpSourceIPv6); ok {
		cfg.Warp.SourceIPv6 = value
	}
	if value, ok := envString(envReasoningEffort); ok {
		cfg.Reasoning.Effort = value
	}
	return nil
}

// applyEnvPassword stores the plaintext password the runtime hashes on startup.
// When the stored hash already verifies against the variable the plaintext is
// left out, so a reload does not re-hash and rewrite the file on every pass.
func applyEnvPassword(cfg *Config, password string) {
	if cfg.WebUI.PasswordHash != "" && VerifyPassword(cfg.WebUI.PasswordHash, password) {
		return
	}
	cfg.WebUI.Password = password
	cfg.WebUI.PasswordHash = ""
}

// envString returns the first of names that is set to a non-empty value.
func envString(names ...string) (string, bool) {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value, true
		}
	}
	return "", false
}

// envList reads a list variable. Entries are separated by commas, semicolons or
// any whitespace, so keys can be pasted one per line from a dashboard.
func envList(name string) []string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\t', '\n', '\r':
			return true
		}
		return false
	})
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		if value := strings.TrimSpace(field); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func envBoolInto(name string, target *bool) error {
	raw, ok := envString(name)
	if !ok {
		return nil
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		*target = true
	case "0", "false", "no", "off":
		*target = false
	default:
		return fmt.Errorf("%s must be true or false, got %q", name, raw)
	}
	return nil
}

func envIntInto(name string, target *int) error {
	raw, ok := envString(name)
	if !ok {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("%s must be an integer, got %q", name, raw)
	}
	*target = value
	return nil
}

// LoadDotEnv reads a .env file and exports every KEY=VALUE it finds that the
// process environment does not already define, so a real variable always wins
// over the file. Blank lines and # comments are ignored, an optional leading
// "export " is accepted, and single- or double-quoted values are unquoted.
// It reports whether the file existed.
func LoadDotEnv(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	number := 0
	for scanner.Scan() {
		number++
		key, value, ok, err := parseDotEnvLine(scanner.Text())
		if err != nil {
			return true, fmt.Errorf("%s:%d: %w", path, number, err)
		}
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return true, fmt.Errorf("%s:%d: %w", path, number, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return true, fmt.Errorf("read %s: %w", path, err)
	}
	return true, nil
}

// parseDotEnvLine parses one line of a .env file. It returns ok=false for
// blank lines and comments.
func parseDotEnvLine(line string) (key, value string, ok bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}
	if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "export\t") {
		line = strings.TrimSpace(line[len("export"):])
	}
	index := strings.IndexByte(line, '=')
	if index <= 0 {
		return "", "", false, fmt.Errorf("expected KEY=VALUE, got %q", line)
	}
	key = strings.TrimSpace(line[:index])
	if !validEnvKey(key) {
		return "", "", false, fmt.Errorf("invalid variable name %q", key)
	}
	value, err = parseDotEnvValue(strings.TrimSpace(line[index+1:]))
	if err != nil {
		return "", "", false, err
	}
	return key, value, true, nil
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for index, r := range key {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case index > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// parseDotEnvValue unquotes a value. Unquoted values keep everything up to a
// trailing " #" comment, which is the form most .env files use.
func parseDotEnvValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	switch raw[0] {
	case '"':
		if len(raw) < 2 || raw[len(raw)-1] != '"' {
			return "", errors.New("unterminated double-quoted value")
		}
		unquoted := raw[1 : len(raw)-1]
		unquoted = strings.ReplaceAll(unquoted, `\n`, "\n")
		unquoted = strings.ReplaceAll(unquoted, `\t`, "\t")
		unquoted = strings.ReplaceAll(unquoted, `\"`, `"`)
		return strings.ReplaceAll(unquoted, `\\`, `\`), nil
	case '\'':
		if len(raw) < 2 || raw[len(raw)-1] != '\'' {
			return "", errors.New("unterminated single-quoted value")
		}
		return raw[1 : len(raw)-1], nil
	}
	if index := strings.Index(raw, " #"); index >= 0 {
		raw = raw[:index]
	}
	return strings.TrimSpace(raw), nil
}
