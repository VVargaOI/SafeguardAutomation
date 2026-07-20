// Code by Claude AI -- Viktor Varga (One Identity)
// Use with OI-RemoteDesktopLauncher 3.0.1+: --use-stdin --args "<full-path>\runas.conf -debug" --cmd "<full-path>\runas.exe"
//
// runas launches a Windows application under a different user account using credentials
// provided by SPP via STDIN. Uses CreateProcessWithLogonW (advapi32.dll) so credentials
// are passed directly to the Win32 API and never appear in process command-line arguments
// visible to other processes or in environment variables or temporary files.

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/google/uuid"
)

var version = "dev"

// LOGON_WITH_PROFILE loads the user's profile into HKEY_USERS.
const logonWithProfile = 0x00000001

type Config struct {
	mode           string // "p" = direct path, "e" = environment variable name
	appPath        string // exe path (mode=p) or env var name containing exe path (mode=e)
	appArgs        string // optional argument string; supports {StdinKey} placeholders
	dumpStdinToLog bool   // if true, write raw STDIN JSON to log (debug only — logs credentials)
}

func defaultConfig() Config {
	return Config{mode: "p"}
}

var (
	modAdvapi32                 = syscall.NewLazyDLL("advapi32.dll")
	procCreateProcessWithLogonW = modAdvapi32.NewProc("CreateProcessWithLogonW")
)

func main() {
	os.Exit(run())
}

// run contains all program logic and returns the process exit code. Using a
// separate function (rather than calling os.Exit directly from many places)
// ensures every deferred cleanup (e.g. logFile.Close, password zeroing) always
// runs on every exit path, since os.Exit does not execute deferred functions.
func run() int {
	// Read the single-line JSON that the Launcher writes to our stdin.
	// The buffer is enlarged (from the bufio.Scanner default of 64KB) since the
	// STDIN payload can contain large templated argument data; without this,
	// scanner.Scan() would fail silently with bufio.ErrTooLong for oversized input.
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var launcherStdinJSON string
	for scanner.Scan() {
		launcherStdinJSON = scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading STDIN: " + err.Error())
		fmt.Println("The runas application will close in 60 seconds..")
		time.Sleep(60 * time.Second)
		return 1
	}

	var stdin map[string]interface{}
	if err := json.Unmarshal([]byte(launcherStdinJSON), &stdin); err != nil {
		fmt.Println("Error parsing STDIN JSON: " + err.Error())
		fmt.Println("The runas application will close in 60 seconds..")
		time.Sleep(60 * time.Second)
		return 1
	}

	if stdin["cli_args"] == nil {
		fmt.Println("Error: required STDIN field missing: cli_args")
		fmt.Println("The runas application will close in 60 seconds..")
		time.Sleep(60 * time.Second)
		return 1
	}

	// cli_args format: "<config-file-path>" or "<config-file-path> -debug"
	configFile, debug := strings.CutSuffix(fmt.Sprint(stdin["cli_args"]), " -debug")

	// --- Logging setup ---
	userProfile := os.Getenv("USERPROFILE")
	logDir := userProfile + `\AppData\Roaming\OneIdentity\OI-SG-RemoteApp-Launcher-Orchestration`
	if _, err := os.Stat(logDir); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(logDir, os.ModePerm); err != nil {
			fmt.Println("Cannot create log directory: " + logDir)
			fmt.Println("The runas application will close in 60 seconds..")
			time.Sleep(60 * time.Second)
			return 1
		}
	}

	logFile, err := os.OpenFile(
		logDir+`\runas_`+time.Now().Format(time.DateOnly)+".log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600,
	)
	if err != nil {
		fmt.Println("Cannot open log file: " + err.Error())
		fmt.Println("The runas application will close in 60 seconds..")
		time.Sleep(60 * time.Second)
		return 1
	}
	defer logFile.Close()

	sessionID := uuid.New().String()
	logLevel := new(slog.LevelVar)
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: logLevel})))

	slog.Info("Starting runas", "version", version, "sessionid", sessionID)
	if debug {
		logLevel.Set(slog.LevelDebug)
		slog.Debug("Log level set to Debug", "sessionid", sessionID)
	} else {
		logLevel.Set(slog.LevelInfo)
	}

	// --- Config file parsing ---
	config := defaultConfig()
	slog.Debug("Config file path: "+configFile, "sessionid", sessionID)

	cfgFile, err := os.Open(configFile)
	if err != nil {
		slog.Error("Cannot open config file", "path", configFile, "error", err.Error(), "sessionid", sessionID)
		return 1
	}
	cfgScanner := bufio.NewScanner(cfgFile)
	for cfgScanner.Scan() {
		line := cfgScanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		slog.Debug("Reading config", "line", line, "sessionid", sessionID)
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			slog.Error("Invalid config line (missing '=')", "line", line, "sessionid", sessionID)
			cfgFile.Close()
			return 1
		}
		switch key {
		case "mode":
			if value != "p" && value != "e" {
				slog.Error("Invalid mode value (must be p or e)", "value", value, "sessionid", sessionID)
				cfgFile.Close()
				return 1
			}
			config.mode = value
		case "appPath":
			config.appPath = value
		case "appArgs":
			config.appArgs = value
		case "dumpStdinToLog":
			config.dumpStdinToLog = value == "true"
		default:
			slog.Error("Unknown config key", "key", key, "sessionid", sessionID)
			cfgFile.Close()
			return 1
		}
	}
	cfgFile.Close()

	if config.appPath == "" {
		slog.Error("Required config missing: appPath", "sessionid", sessionID)
		return 1
	}

	if config.dumpStdinToLog {
		slog.Debug("STDIN dump", "json", launcherStdinJSON, "sessionid", sessionID)
	}

	// --- Resolve application path ---
	var resolvedPath string
	if config.mode == "p" {
		resolvedPath = config.appPath
	} else {
		resolvedPath = os.Getenv(config.appPath)
		if resolvedPath == "" {
			slog.Error("Environment variable not set or empty", "var", config.appPath, "sessionid", sessionID)
			return 1
		}
	}
	slog.Debug("Resolved app path", "path", resolvedPath, "sessionid", sessionID)

	// --- Validate required STDIN fields ---
	for _, field := range []string{"Target.AccountName", "Target.AccountDomainName", "password"} {
		if stdin[field] == nil {
			slog.Error("Required STDIN field missing", "field", field, "sessionid", sessionID)
			return 1
		}
	}

	accountName := fmt.Sprint(stdin["Target.AccountName"])
	domainName := fmt.Sprint(stdin["Target.AccountDomainName"])

	// Take the password as a mutable byte slice so it can be zeroed after use,
	// then remove it from the parsed map so the original string isn't retained there.
	passwordBytes := []byte(fmt.Sprint(stdin["password"]))
	stdin["password"] = nil
	defer zeroBytes(passwordBytes)

	asset := ""
	if stdin["Target.AssetNetworkAddress"] != nil {
		asset = fmt.Sprint(stdin["Target.AssetNetworkAddress"])
	}

	// --- Resolve {StdinKey} templates in appArgs ---
	// Note: avoid placing {password} or other secrets in appArgs — they appear in the child
	// process's command line, which is visible to other processes via the Windows API.
	resolvedArgs, err := resolveTemplate(config.appArgs, stdin)
	if err != nil {
		slog.Error("Failed to resolve appArgs template", "error", err.Error(), "sessionid", sessionID)
		return 1
	}

	slog.Info("Launching application",
		"path", resolvedPath,
		"args", resolvedArgs,
		"user", accountName+"@"+domainName,
		"asset", asset,
		"sessionid", sessionID,
	)

	if err := runAs(resolvedPath, resolvedArgs, accountName, domainName, passwordBytes, sessionID); err != nil {
		slog.Error("Failed to launch application", "error", err.Error(), "sessionid", sessionID)
		return 1
	}

	slog.Info("Application launched successfully", "sessionid", sessionID)
	return 0
}

// zeroBytes overwrites b with zeros. Best-effort defense-in-depth for credential
// material held in a mutable byte slice; the Go runtime may still have made
// copies (e.g. via string conversions) that this cannot reach.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// resolveTemplate replaces {Key} placeholders in s with the corresponding values from stdin.
// Substituted values are rejected if they contain a double-quote character: since the
// result is embedded directly into a Win32 command line (see quoteArg/runAs), an
// attacker-influenced value containing '"' could otherwise break out of argument
// quoting and inject additional command-line arguments.
func resolveTemplate(s string, stdin map[string]interface{}) (string, error) {
	var sb strings.Builder
	for {
		start := strings.Index(s, "{")
		if start == -1 {
			sb.WriteString(s)
			break
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			sb.WriteString(s)
			break
		}
		end += start
		key := s[start+1 : end]
		val, ok := stdin[key]
		if !ok || val == nil {
			return "", fmt.Errorf("template key not found in STDIN: %s", key)
		}
		valStr := fmt.Sprint(val)
		if strings.Contains(valStr, `"`) {
			return "", fmt.Errorf("template value for key %q contains an illegal '\"' character", key)
		}
		sb.WriteString(s[:start])
		sb.WriteString(valStr)
		s = s[end+1:]
	}
	return sb.String(), nil
}

// quoteArg quotes a single command-line argument using the Windows argument-quoting
// rules (as consumed by CommandLineToArgvW / the Win32 C runtime), so that embedded
// double quotes, backslashes, and whitespace cannot be misinterpreted as argument
// separators or used to inject additional arguments.
func quoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\v\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range s {
		switch r {
		case '\\':
			slashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes*2+1))
			b.WriteByte('"')
			slashes = 0
		default:
			if slashes > 0 {
				b.WriteString(strings.Repeat(`\`, slashes))
				slashes = 0
			}
			b.WriteRune(r)
		}
	}
	if slashes > 0 {
		b.WriteString(strings.Repeat(`\`, slashes*2))
	}
	b.WriteByte('"')
	return b.String()
}

// runAs launches exePath as the specified Windows user via CreateProcessWithLogonW.
// Credentials are passed directly to the Win32 API; they are never written to disk,
// environment variables, or the child process's command line. password is supplied
// as a mutable byte slice so the caller can zero it once this call returns.
func runAs(exePath, args, username, domain string, password []byte, sessionID string) error {
	usernamePtr, err := syscall.UTF16PtrFromString(username)
	if err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	domainPtr, err := syscall.UTF16PtrFromString(domain)
	if err != nil {
		return fmt.Errorf("invalid domain: %w", err)
	}
	// Build a mutable UTF16 buffer for the password ourselves (rather than
	// syscall.UTF16PtrFromString) so it can be zeroed immediately after the call.
	passwordSlice, err := syscall.UTF16FromString(string(password))
	if err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}
	defer zeroUint16s(passwordSlice)
	exePtr, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return fmt.Errorf("invalid exe path: %w", err)
	}

	// Build the full command line; exe path goes first as argv[0]. exePath is
	// quoted using proper Windows argument-quoting rules (see quoteArg) rather
	// than naive concatenation, to prevent embedded quotes from splitting or
	// injecting additional arguments.
	cmdLine := quoteArg(exePath)
	if args != "" {
		cmdLine += " " + args
	}
	// CreateProcessWithLogonW may modify the command-line buffer, so use a mutable slice.
	cmdLineSlice, err := syscall.UTF16FromString(cmdLine)
	if err != nil {
		return fmt.Errorf("invalid command line: %w", err)
	}

	si := new(syscall.StartupInfo)
	si.Cb = uint32(unsafe.Sizeof(*si))
	pi := new(syscall.ProcessInformation)

	slog.Debug("Calling CreateProcessWithLogonW",
		"username", username, "domain", domain,
		"exe", exePath, "cmdline", cmdLine,
		"sessionid", sessionID,
	)

	r1, _, winErr := procCreateProcessWithLogonW.Call(
		uintptr(unsafe.Pointer(usernamePtr)),
		uintptr(unsafe.Pointer(domainPtr)),
		uintptr(unsafe.Pointer(&passwordSlice[0])),
		uintptr(logonWithProfile),
		uintptr(unsafe.Pointer(exePtr)),
		uintptr(unsafe.Pointer(&cmdLineSlice[0])),
		0, // creationFlags: default (inherits parent's console/environment)
		0, // environment:   NULL = inherit parent
		0, // currentDirectory: NULL = inherit parent
		uintptr(unsafe.Pointer(si)),
		uintptr(unsafe.Pointer(pi)),
	)
	if r1 == 0 {
		return fmt.Errorf("CreateProcessWithLogonW failed: %w", winErr)
	}

	slog.Debug("Process created", "pid", pi.ProcessId, "sessionid", sessionID)

	// We don't wait for the child — close the handles immediately.
	syscall.CloseHandle(pi.Process)
	syscall.CloseHandle(pi.Thread)

	return nil
}

// zeroUint16s overwrites s with zeros. Best-effort defense-in-depth for the
// password buffer passed to CreateProcessWithLogonW.
func zeroUint16s(s []uint16) {
	for i := range s {
		s[i] = 0
	}
}
