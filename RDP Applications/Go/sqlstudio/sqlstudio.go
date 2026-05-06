// Code by Viktor Varga (One Identity), UIA rewrite by Claude Sonnet 4.6
// Use with the OI-RemoteDesktopLauncher 3.0.1 or later: --use-stdin --args "<full-path>\sqlstudio.conf -debug" --cmd "<full-path>\sqlstudio.exe"
//
// sqlstudio launches SQL Server Management Studio (SSMS) with Entra ID Interactive authentication
// and automates the Entra ID WAM (Windows Account Manager) login dialog using Windows UI Automation.
//
// SSMS 22+ uses the Windows Account Manager (WAM) broker for Entra ID authentication instead of
// an embedded WebView2 dialog. The WAM login UI is hosted in ShellAppRuntime (ApplicationFrameWindow)
// and is driven via the Windows UI Automation accessibility API.

package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	dumpStdinToLog    bool
	loginActions      string
	browserInputDelay int
	ssmspath          string
	uiaWaitTimeout    int
}

func defaultConfig() Config {
	return Config{
		dumpStdinToLog:    false,
		loginActions:      "s::#i0118::{password}||c::#idSIButton9::10||o::#idTxtBx_SAOTCC_OTC::{Target.TotpCodes}::3||c::#idSubmit_SAOTCC_Continue::10||c::#idBtn_Back::5",
		browserInputDelay: 1000,
		uiaWaitTimeout:    60,
	}
}

type resolvedAction struct {
	kind     string
	selector string
	value    string
	waitSecs int
	optional bool
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	var launcherStdinJSON string
	for scanner.Scan() {
		launcherStdinJSON = scanner.Text()
	}
	err := scanner.Err()
	if err != nil {
		fmt.Println(err.Error())
		_, err := fmt.Scanf("%s")
		if err != nil {
			fmt.Println("Error occured while reading STDIN.")
			fmt.Println("The sqlstudio application will close in 60 seconds..")
			time.Sleep(time.Duration(60) * time.Second)
			os.Exit(1)
		}
	}

	var launcherStdin map[string]interface{}
	json.Unmarshal([]byte(launcherStdinJSON), &launcherStdin)
	configFile, debug := strings.CutSuffix(launcherStdin["cli_args"].(string), " -debug")

	userProfileDir := os.Getenv("USERPROFILE")
	logDir := userProfileDir + `\AppData\Roaming\OneIdentity\OI-SG-RemoteApp-Launcher-Orchestration`
	if _, err := os.Stat(logDir); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(logDir, os.ModePerm)
		if err != nil {
			fmt.Println("Cannot create log directory: " + logDir)
			fmt.Println("The sqlstudio application will close in 60 seconds..")
			time.Sleep(time.Duration(60) * time.Second)
			os.Exit(1)
		}
	}

	f, err := os.OpenFile(logDir+`\sqlstudio_`+time.Now().Format(time.DateOnly)+".log", os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		fmt.Println(err)
		fmt.Println("Cannot create or open log file.")
		fmt.Println("The sqlstudio application will close in 60 seconds..")
		_, err := fmt.Scanf("%s")
		if err != nil {
			defer f.Close()
			time.Sleep(time.Duration(60) * time.Second)
			os.Exit(1)
		}
	}
	defer f.Close()

	sessionID := uuid.New().String()
	var logLevel = new(slog.LevelVar)
	logger := slog.NewTextHandler(f, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(logger))

	slog.Info("Starting sqlstudio..", "sessionid", sessionID)
	if debug {
		logLevel.Set(slog.LevelDebug)
		slog.Debug("Loglevel set to Debug", "sessionid", sessionID)
	} else {
		logLevel.Set(slog.LevelInfo)
	}

	slog.Debug("Config file path: "+configFile, "sessionid", sessionID)
	config := defaultConfig()

	readFile, err := os.Open(configFile)
	if err != nil {
		slog.Error("Error occured while opening config file: "+launcherStdin["cli_args"].(string), "sessionid", sessionID)
		slog.Error("Error: "+err.Error(), "sessionid", sessionID)
		os.Exit(1)
	}
	fileScanner := bufio.NewScanner(readFile)
	fileScanner.Split(bufio.ScanLines)
	for fileScanner.Scan() {
		if !strings.HasPrefix(fileScanner.Text(), "#") && fileScanner.Text() != "" && strings.TrimSpace(fileScanner.Text()) != "" {
			slog.Debug("Reading configuration file", "config", fileScanner.Text(), "sessionid", sessionID)
			switch strings.Split(fileScanner.Text(), "=")[0] {
			case "dumpStdinToLog":
				config.dumpStdinToLog, err = strconv.ParseBool(strings.Split(fileScanner.Text(), "=")[1])
				if err != nil {
					slog.Error("Error occured while parsing configuration: "+strings.Split(fileScanner.Text(), "=")[1]+"to boolean.", "sessionid", sessionID)
					os.Exit(1)
				}
			case "loginActions":
				_, loginActionsValue, _ := strings.Cut(fileScanner.Text(), "=")
				config.loginActions = loginActionsValue
			case "browserInputDelay":
				config.browserInputDelay, err = strconv.Atoi(strings.Split(fileScanner.Text(), "=")[1])
				if err != nil {
					slog.Error("Error occured while parsing configuration: "+strings.Split(fileScanner.Text(), "=")[1]+"to int.", "sessionid", sessionID)
					os.Exit(1)
				}
			case "uiaWaitTimeout":
				config.uiaWaitTimeout, err = strconv.Atoi(strings.Split(fileScanner.Text(), "=")[1])
				if err != nil {
					slog.Error("Error occured while parsing configuration: "+strings.Split(fileScanner.Text(), "=")[1]+"to int.", "sessionid", sessionID)
					os.Exit(1)
				}
			case "ssmspath":
				_, ssmspathValue, _ := strings.Cut(fileScanner.Text(), "=")
				config.ssmspath = ssmspathValue
			case "chromedp_logging", "chromedp_queryOption", "ssmsDebugPort":
				slog.Debug("Ignoring legacy configuration key: "+strings.Split(fileScanner.Text(), "=")[0], "sessionid", sessionID)
			default:
				slog.Error("Unknown configuration name: "+strings.Split(fileScanner.Text(), "=")[0], "sessionid", sessionID)
				os.Exit(1)
			}
		}
	}
	readFile.Close()

	if config.dumpStdinToLog {
		slog.Debug("STDIN: "+launcherStdinJSON, "sessionid", sessionID)
	}

	if config.ssmspath == "" {
		slog.Error("Required configuration missing: ssmspath", "sessionid", sessionID)
		os.Exit(1)
	}

	if launcherStdin["Target.AssetNetworkAddress"] == nil {
		slog.Error("Required STDIN value missing: Target.AssetNetworkAddress", "sessionid", sessionID)
		os.Exit(1)
	}
	if launcherStdin["Target.AccountName"] == nil {
		slog.Error("Required STDIN value missing: Target.AccountName", "sessionid", sessionID)
		os.Exit(1)
	}
	if launcherStdin["Target.AccountDomainName"] == nil {
		slog.Error("Required STDIN value missing: Target.AccountDomainName", "sessionid", sessionID)
		os.Exit(1)
	}
	targetAddress := fmt.Sprint(launcherStdin["Target.AssetNetworkAddress"])
	accountName := fmt.Sprint(launcherStdin["Target.AccountName"]) + "@" + fmt.Sprint(launcherStdin["Target.AccountDomainName"])

	ssmsExe := strings.TrimSuffix(config.ssmspath, `\`) + `\ssms.exe`
	slog.Debug("Launching SSMS", "path", ssmsExe, "server", targetAddress, "user", accountName, "sessionid", sessionID)

	cmd := exec.Command(ssmsExe, "-S", targetAddress, "-U", accountName, "-A", "ActiveDirectoryInteractive")
	if err := cmd.Start(); err != nil {
		slog.Error("Error launching SSMS", "path", ssmsExe, "error", err.Error(), "sessionid", sessionID)
		os.Exit(1)
	}
	slog.Debug("SSMS process started", "pid", cmd.Process.Pid, "sessionid", sessionID)

	slog.Debug("Parsing loginActions", "actions", config.loginActions, "sessionid", sessionID)
	actions := parseActions(config.loginActions, launcherStdin, config.uiaWaitTimeout, sessionID)
	slog.Debug("Parsed "+strconv.Itoa(len(actions))+" UIA actions", "sessionid", sessionID)

	slog.Info("Starting UIA login automation", "sessionid", sessionID)
	if uiaErr := runUIAAutomation(actions, config.browserInputDelay, sessionID); uiaErr != nil {
		slog.Error("UIA automation failed", "error", uiaErr.Error(), "sessionid", sessionID)
		os.Exit(1)
	}

	slog.Info("UIA login automation completed successfully", "sessionid", sessionID)
	os.Exit(0)
}

func stripSelector(s string) string {
	return strings.TrimPrefix(s, "#")
}

func parseActions(loginActions string, stdin map[string]interface{}, defaultTimeoutSecs int, sessionID string) []resolvedAction {
	parts := strings.Split(loginActions, "||")
	var result []resolvedAction

	for _, part := range parts {
		fields := strings.Split(part, "::")
		if len(fields) < 2 {
			slog.Error("Invalid action format (expected at least type::selector)", "action", part, "sessionid", sessionID)
			os.Exit(1)
		}

		kind := fields[0]
		aid := stripSelector(fields[1])

		resolveKey := func(raw string) string {
			key := strings.TrimPrefix(strings.TrimSuffix(raw, "}"), "{")
			if stdin[key] == nil {
				slog.Error("Required STDIN value missing", "key", key, "sessionid", sessionID)
				os.Exit(1)
			}
			return fmt.Sprint(stdin[key])
		}

		switch kind {
		case "c":
			clickTimeout := 10
			if len(fields) >= 3 {
				if t, err := strconv.Atoi(fields[2]); err == nil {
					clickTimeout = t
				}
			}
			result = append(result, resolvedAction{
				kind:     "c",
				selector: aid,
				waitSecs: clickTimeout,
				optional: true,
			})
		case "v", "s":
			if len(fields) < 3 {
				slog.Error("Action requires a value field", "action", part, "sessionid", sessionID)
				os.Exit(1)
			}
			result = append(result, resolvedAction{
				kind:     kind,
				selector: aid,
				value:    resolveKey(fields[2]),
				waitSecs: defaultTimeoutSecs,
			})
		case "o":
			if len(fields) < 3 {
				slog.Error("TOTP action requires a value field", "action", part, "sessionid", sessionID)
				os.Exit(1)
			}
			minSecsBeforeExpiry := 0
			if len(fields) >= 4 {
				minSecsBeforeExpiry, _ = strconv.Atoi(fields[3])
			}
			totpJSON := resolveKey(fields[2])
			otp := resolveTOTP(totpJSON, minSecsBeforeExpiry, sessionID)
			result = append(result, resolvedAction{
				kind:     "s",
				selector: aid,
				value:    otp,
				waitSecs: defaultTimeoutSecs,
			})
		default:
			slog.Error("Unknown action type", "type", kind, "action", part, "sessionid", sessionID)
			os.Exit(1)
		}
	}

	return result
}

func resolveTOTP(totpJSON string, minSecsBeforeExpiry int, sessionID string) string {
	var otps []map[string]interface{}
	if err := json.Unmarshal([]byte(totpJSON), &otps); err != nil {
		slog.Error("Error parsing TOTP JSON", "error", err.Error(), "sessionid", sessionID)
		os.Exit(1)
	}

	for idx, otp := range otps {
		currentUnixTime := time.Now().Unix()
		unixTime := int64(otp["UnixTime"].(float64))
		period := int64(otp["Period"].(float64))
		diff := unixTime + period - currentUnixTime

		slog.Debug("Examining TOTP code", "index", idx+1, "expiry_diff_secs", diff, "min_required", minSecsBeforeExpiry, "sessionid", sessionID)

		if diff >= int64(minSecsBeforeExpiry) {
			code := otp["Code"].(string)
			slog.Debug("Found valid TOTP code", "expiry_in_secs", diff, "sessionid", sessionID)
			return code
		}
		if diff < 0 {
			slog.Debug("TOTP code already expired, checking next", "diff", diff, "sessionid", sessionID)
		} else {
			slog.Debug("TOTP code too close to expiry, checking next", "diff", diff, "min", minSecsBeforeExpiry, "sessionid", sessionID)
		}
	}

	log.Fatalln("[TOTP] No valid TOTP code found")
	return ""
}

// utf16leBase64 encodes a string as Base64(UTF-16LE), which is the format required by
// PowerShell's -EncodedCommand parameter. Works correctly for ASCII-range scripts.
func utf16leBase64(s string) string {
	runes := []rune(s)
	buf := make([]byte, 0, len(runes)*2)
	for _, r := range runes {
		buf = append(buf, byte(r), byte(r>>8))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// runUIAAutomation executes the resolved login actions against the WAM dialog via a single
// PowerShell process that uses the Windows UI Automation COM API.
//
// Security model:
//   - The script logic (no secrets) is passed via -EncodedCommand on the command line.
//     It is base64-decodable by anyone who can see the process list, but contains NO secrets.
//   - Secrets are written as one base64(JSON) line to the anonymous stdin pipe.
//     The pipe buffer is a kernel object not readable by other processes without SeDebugPrivilege.
//   - No environment variables carry secrets. No temp files are written.
func runUIAAutomation(actions []resolvedAction, delayMs int, sessionID string) error {
	// Build a secrets map: key = "k<n>" → plaintext value
	secretsData := map[string]string{}
	skIdx := 0
	secretKeys := make([]string, len(actions))
	for i, a := range actions {
		if a.kind == "s" || a.kind == "v" {
			key := fmt.Sprintf("k%d", skIdx)
			secretKeys[i] = key
			secretsData[key] = a.value
			skIdx++
		}
	}

	// Encode secrets as base64(JSON) — written as the first line of stdin.
	secretsJSON, _ := json.Marshal(secretsData)
	secretsB64 := base64.StdEncoding.EncodeToString(secretsJSON)

	var sb strings.Builder

	// Script reads secrets from stdin as its very first action.
	// stdin is the anonymous pipe — not visible in the process list.
	sb.WriteString(`$secrets = [System.Text.Encoding]::UTF8.GetString(
    [Convert]::FromBase64String([Console]::In.ReadLine())) | ConvertFrom-Json
`)

	sb.WriteString(`Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Windows.Forms
$root = [System.Windows.Automation.AutomationElement]::RootElement

function WaitForAID($aid, $secs) {
    $maxIter = $secs * 4
    for ($i = 0; $i -lt $maxIter; $i++) {
        $cond = New-Object System.Windows.Automation.PropertyCondition(
            [System.Windows.Automation.AutomationElement]::AutomationIdProperty, $aid)
        $el = $root.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $cond)
        if ($el -ne $null) { return $el }
        Start-Sleep -Milliseconds 250
    }
    return $null
}

function InvokeEl($el) {
    try {
        $ip = $el.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern)
        $ip.Invoke()
        return $true
    } catch {}
    try {
        $el.SetFocus()
        Start-Sleep -Milliseconds 100
        [System.Windows.Forms.SendKeys]::SendWait(" ")
        return $true
    } catch {}
    return $false
}

function SetElValue($el, $secretKey) {
    $val = $secrets.$secretKey
    try {
        $vp = $el.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern)
        $vp.SetValue($val)
        return $true
    } catch {}
    try {
        $el.SetFocus()
        Start-Sleep -Milliseconds 200
        [System.Windows.Forms.SendKeys]::SendWait("^a")
        Start-Sleep -Milliseconds 100
        foreach ($c in $val.ToCharArray()) {
            $s = [string]$c
            if ($s -match '[+^%~()\[\]{}]') { $s = "{$s}" }
            [System.Windows.Forms.SendKeys]::SendWait($s)
        }
        return $true
    } catch {}
    return $false
}

`)

	for i, a := range actions {
		switch a.kind {
		case "c":
			sb.WriteString(fmt.Sprintf(
				"$el%d = WaitForAID \"%s\" %d\n"+
					"if ($el%d -ne $null) {\n"+
					"    InvokeEl $el%d | Out-Null\n"+
					"    Write-Host \"Clicked: %s\"\n"+
					"} else {\n"+
					"    Write-Host \"WARNING: optional element not found: %s\"\n"+
					"}\n",
				i, a.selector, a.waitSecs,
				i,
				i,
				a.selector,
				a.selector,
			))
		case "s", "v":
			sb.WriteString(fmt.Sprintf(
				"$el%d = WaitForAID \"%s\" %d\n"+
					"if ($el%d -eq $null) { Write-Host \"ERROR: required element not found: %s\"; exit 1 }\n"+
					"if (-not (SetElValue $el%d \"%s\")) { Write-Host \"ERROR: failed to set value on: %s\"; exit 1 }\n"+
					"Write-Host \"Set value on: %s\"\n",
				i, a.selector, a.waitSecs,
				i, a.selector,
				i, secretKeys[i], a.selector,
				a.selector,
			))
		}

		if delayMs > 0 && a.kind == "c" {
			sb.WriteString(fmt.Sprintf("Start-Sleep -Milliseconds %d\n", delayMs))
		}
	}

	sb.WriteString("Write-Host \"All UIA actions completed\"\n")

	scriptText := sb.String()
	slog.Debug("Built UIA PowerShell script", "length_bytes", len(scriptText), "sessionid", sessionID)

	// Deliver script via -EncodedCommand (UTF-16LE base64). The command line shows the
	// encoded script — decodable but contains NO secrets. Secrets arrive via stdin only.
	psCmd := exec.Command("powershell", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", utf16leBase64(scriptText))
	psCmd.Env = os.Environ()
	stdinPipe, pipeErr := psCmd.StdinPipe()
	if pipeErr != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", pipeErr)
	}
	var outBuf bytes.Buffer
	psCmd.Stdout = &outBuf
	psCmd.Stderr = &outBuf

	if startErr := psCmd.Start(); startErr != nil {
		return fmt.Errorf("failed to start powershell: %w", startErr)
	}

	// Write secrets as the single first line of stdin, then close.
	// The script reads this line via [Console]::In.ReadLine() and never needs stdin again.
	_, _ = fmt.Fprintln(stdinPipe, secretsB64)
	stdinPipe.Close()

	runErr := psCmd.Wait()

	outStr := strings.TrimSpace(outBuf.String())
	if outStr != "" {
		slog.Debug("UIA automation output:\n"+outStr, "sessionid", sessionID)
	}

	return runErr
}
