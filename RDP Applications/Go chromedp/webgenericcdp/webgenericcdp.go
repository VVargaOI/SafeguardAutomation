// Author: Viktor Varga (One Identity). Certain code snippets are by Ferenc Sipos (One Identity) or Richard Hosgood (One Identity).
// Use with the OI-RemoteDesktopLauncher 3.0.1 or later: --use-stdin --args "<full-path>\webgenericcdp_webapp.conf -debug" --cmd "<full-path>\webgenericcdp.exe"

package main

// Importing packages needed by the program
import (
	// Standard library packages
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	// Driver to talk to Chrome-based browsers leveraging the
	// Chrome DevTools protocol
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

type Config struct {
	dumpStdinToLog    bool
	chromedp_logging  string
	url               string
	browser           string
	loginActions      string
	splitCharacters   string
	browserInputDelay int
	browser_incognito bool
	browser_insecure  bool
	browser_kiosk     bool
	user_data_dir     string
	basicAuthUsername string
}

func defaultConfig() Config {
	return Config{
		dumpStdinToLog:   false,   // WARNING, this contains the clear-text password
		chromedp_logging: "error", // error|log|debug
		//url               //has no default
		browser: "chrome", // Must be chrome or edge
		//loginActions		//has no default
		splitCharacters:   "\\@", // List of characters which may be used on concatenated values like UPN or down-level logon name
		browserInputDelay: 0,     // If set (in milliseconds), the code does not wait until the element is presented by the browser, but perfoms the next action when the configured delay passed
		browser_incognito: true,
		browser_insecure:  false, // Ignore certificate errors
		browser_kiosk:     false,
		//user_data_dir		//has no default
		basicAuthUsername: "false",
	}
}

func main() {

	// Read input from STDIN
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
			fmt.Println("The webgenericcdp application will close in 60 seconds..")
			time.Sleep(time.Duration(60) * time.Second)
			os.Exit(1)
		}
	}

	// Parse JSON input
	var launcherStdin map[string]interface{}
	json.Unmarshal([]byte(launcherStdinJSON), &launcherStdin)

	configFile, debug := strings.CutSuffix(launcherStdin["cli_args"].(string), " -debug")

	// Initialize log file
	userProfileDir := os.Getenv("USERPROFILE")
	logDir := userProfileDir + "\\AppData\\Roaming\\OneIdentity\\OI-SG-RemoteApp-Launcher-Orchestration"
	if _, err := os.Stat(logDir); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(logDir, os.ModePerm)
		if err != nil {
			fmt.Println("Cannot create log directory: " + logDir)
			fmt.Println("The webgenericcdp application will close in 60 seconds..")
			time.Sleep(time.Duration(60) * time.Second)
			os.Exit(1)
		}
	}

	f, err := os.OpenFile(logDir+"\\webgenericcdp_"+time.Now().Format(time.DateOnly)+".log", os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		fmt.Println(err)
		fmt.Println("Cannot create or open log file.")
		fmt.Println("The webgenericcdp application will close in 60 seconds..")
		_, err := fmt.Scanf("%s")
		if err != nil {
			defer f.Close()
			time.Sleep(time.Duration(60) * time.Second)
			os.Exit(1)
		}
	}
	defer f.Close()

	uuid := uuid.New().String()
	var logLevel = new(slog.LevelVar)
	logger := slog.NewTextHandler(f, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(logger))

	slog.Info("Starting webgenericcdp..", "sessionid", uuid)
	if debug {
		logLevel.Set(slog.LevelDebug)
		slog.Debug("Loglevel set to Debug", "sessionid", uuid)
	} else {
		logLevel.Set(slog.LevelInfo)
	}

	slog.Debug("Config file path: "+configFile, "sessionid", uuid)

	config := defaultConfig()

	// Read configuration from file
	readFile, err := os.Open(configFile)
	if err != nil {
		slog.Error("Error occured while opening config file: "+launcherStdin["cli_args"].(string), "sessionid", uuid)
		slog.Error("Error: "+err.Error(), "sessionid", uuid)
		os.Exit(1)
	}
	fileScanner := bufio.NewScanner(readFile)

	fileScanner.Split(bufio.ScanLines)

	for fileScanner.Scan() {
		if !strings.HasPrefix(fileScanner.Text(), "#") && fileScanner.Text() != "" && strings.TrimSpace(fileScanner.Text()) != "" {
			slog.Debug("Reading configuration file", "config", fileScanner.Text(), "sessionid", uuid)
			switch strings.Split(fileScanner.Text(), "=")[0] {
			case "dumpStdinToLog":
				config.dumpStdinToLog, err = strconv.ParseBool(strings.Split(fileScanner.Text(), "=")[1])
				if err != nil {
					slog.Error("Error occured while parsing configuration: "+strings.Split(fileScanner.Text(), "=")[1]+"to boolean. Accepted string values: \"1\", \"t\", \"T\", \"TRUE\", \"true\", \"True\", \"0\", \"f\", \"F\", \"FALSE\", \"false\", \"False\"", "sessionid", uuid)
					slog.Error("Error: "+err.Error(), "sessionid", uuid)
					os.Exit(1)
				}
			case "chromedp_logging":
				config.chromedp_logging = strings.Split(fileScanner.Text(), "=")[1]
			case "url":
				config.url = strings.Split(fileScanner.Text(), "=")[1]
			case "browser":
				config.browser = strings.Split(fileScanner.Text(), "=")[1]
			case "loginActions":
				config.loginActions = strings.Split(fileScanner.Text(), "=")[1]
			case "splitCharacters":
				_, splitChars, _ := strings.Cut(fileScanner.Text(), "=")
				config.splitCharacters = splitChars
			case "browserInputDelay":
				config.browserInputDelay, err = strconv.Atoi(strings.Split(fileScanner.Text(), "=")[1])
				if err != nil {
					slog.Error("Error occured while parsing configuration: "+strings.Split(fileScanner.Text(), "=")[1]+"to int.", "sessionid", uuid)
					slog.Error("Error: "+err.Error(), "sessionid", uuid)
					os.Exit(1)
				}
			case "browser_incognito":
				config.browser_incognito, err = strconv.ParseBool(strings.Split(fileScanner.Text(), "=")[1])
				if err != nil {
					slog.Error("Error occured while parsing configuration: "+strings.Split(fileScanner.Text(), "=")[1]+"to boolean. Accepted string values: \"1\", \"t\", \"T\", \"TRUE\", \"true\", \"True\", \"0\", \"f\", \"F\", \"FALSE\", \"false\", \"False\"", "sessionid", uuid)
					slog.Error("Error: "+err.Error(), "sessionid", uuid)
					os.Exit(1)
				}
			case "browser_insecure":
				config.browser_insecure, err = strconv.ParseBool(strings.Split(fileScanner.Text(), "=")[1])
				if err != nil {
					slog.Error("Error occured while parsing configuration: "+strings.Split(fileScanner.Text(), "=")[1]+"to boolean. Accepted string values: \"1\", \"t\", \"T\", \"TRUE\", \"true\", \"True\", \"0\", \"f\", \"F\", \"FALSE\", \"false\", \"False\"", "sessionid", uuid)
					slog.Error("Error: "+err.Error(), "sessionid", uuid)
					os.Exit(1)
				}
			case "browser_kiosk":
				config.browser_kiosk, err = strconv.ParseBool(strings.Split(fileScanner.Text(), "=")[1])
				if err != nil {
					slog.Error("Error occured while parsing configuration: "+strings.Split(fileScanner.Text(), "=")[1]+"to boolean. Accepted string values: \"1\", \"t\", \"T\", \"TRUE\", \"true\", \"True\", \"0\", \"f\", \"F\", \"FALSE\", \"false\", \"False\"", "sessionid", uuid)
					slog.Error("Error: "+err.Error(), "sessionid", uuid)
					os.Exit(1)
				}
			case "user_data_dir":
				config.user_data_dir = strings.Split(fileScanner.Text(), "=")[1]
			case "basicAuthUsername":
				config.basicAuthUsername = strings.Split(fileScanner.Text(), "=")[1]
			default:
				slog.Error("Unknown configuration name: "+strings.Split(fileScanner.Text(), "=")[0], "sessionid", uuid)
				os.Exit(1)
			}
		}
	}

	readFile.Close()

	// Is there anything missing from config?
	if config == (Config{}) {
		slog.Error("Something is missing from the configuration. Config (including default values):", "sessionid", uuid)
		configDump, err := json.Marshal(config)
		if err != nil {
			slog.Error("Can't convert config struct into JSON")
			os.Exit(1)
		}
		slog.Error(string(configDump), "sessionid", uuid)
		os.Exit(1)
	}

	if config.dumpStdinToLog {
		slog.Debug("STDIN: "+launcherStdinJSON, "sessionid", uuid)
	}

	slog.Debug("Setting up browser options", "sessionid", uuid)
	opts := append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("hide-scrollbars", false),
		chromedp.Flag("mute-audio", false),
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("window-size", "1280,800"),
	)
	if config.browser == "edge" {
		edgePath := "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe"
		slog.Debug("Using Edge", "path", edgePath, "sessionid", uuid)
		opts = append(opts,
			chromedp.ExecPath(edgePath),
		)
	}
	if config.browser_incognito {
		slog.Debug("Using Incognito mode", "sessionid", uuid)
		opts = append(opts,
			chromedp.Flag("incognito", true),
		)
	}
	if config.browser_insecure {
		slog.Debug("Ignore Certificate Errors", "sessionid", uuid)
		opts = append(opts,
			chromedp.Flag("ignore-certificate-errors", true),
		)
	}
	if config.browser_kiosk {
		slog.Debug("Using Kiosk mode", "sessionid", uuid)
		opts = append(opts,
			chromedp.Flag("kiosk", true),
		)
	}

	if config.user_data_dir != "" {
		slog.Debug("Setting browser profile directory", "UserDataDir", config.user_data_dir, "sessionid", uuid)
		profileDir := config.user_data_dir
		if strings.Contains(config.user_data_dir, "%AppData%") {
			profileDir = strings.Replace(config.user_data_dir, "%AppData%", (userProfileDir + "\\AppData\\Roaming"), 1)
			slog.Debug("Replace %AppData%..", "Profile_directory", profileDir, "sessionid", uuid)
		}
		if _, err := os.Stat(profileDir); errors.Is(err, os.ErrNotExist) {
			err := os.Mkdir(profileDir, os.ModePerm)
			if err != nil {
				fmt.Println("Cannot create profile directory: " + profileDir)
				fmt.Println("The webgenericcdp application will close in 60 seconds..")
				time.Sleep(time.Duration(60) * time.Second)
				os.Exit(1)
			}
		}
		opts = append(opts,
			chromedp.UserDataDir(profileDir),
		)

	}

	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	var runCtx context.Context

	switch {
	case config.chromedp_logging == "error":
		runCtx, _ = chromedp.NewContext(allocCtx, chromedp.WithErrorf(slog.Error))
	case config.chromedp_logging == "info":
		runCtx, _ = chromedp.NewContext(allocCtx, chromedp.WithBrowserOption(chromedp.WithBrowserLogf(slog.Info)))
	case config.chromedp_logging == "debug":
		runCtx, _ = chromedp.NewContext(allocCtx, chromedp.WithDebugf(slog.Debug))
	default:
		slog.Error("Invalid chromedp logging configuration", "configuration", config.chromedp_logging, "accepted values", "error|info|debug", "sessionid", uuid)
		os.Exit(1)

	}

	// Check if URL contains any value from Safeguard
	urlmatch, _ := regexp.MatchString((".*{.*}"), config.url)
	if urlmatch {
		slog.Debug("Safeguard value found in url", "sessionid", uuid)
		urlsubs := strings.SplitN(config.url, "{", 2)
		urlsubs2 := strings.SplitN(urlsubs[1], "}", 2)
		config.url = urlsubs[0] + fmt.Sprint(launcherStdin[urlsubs2[0]]) + urlsubs2[1]
		slog.Debug("Safeguard value inserted", "url", config.url, "sessionid", uuid)
	}

	// Declare tasklist
	taskList := []chromedp.Action{}
	if config.basicAuthUsername != "false" {
		slog.Debug("Basic Authentication", "username", config.basicAuthUsername, "sessionid", uuid)
		slog.Debug("Building chromedp taskList..", "sessionid", uuid)

		var basicAuthUsername string

		if strings.HasPrefix(config.basicAuthUsername, "{") && strings.HasSuffix(config.basicAuthUsername, "}") {
			// Trim key names from basicAuthUsername so that we can use them as keys for the values received from Safeguard via STDIN
			slog.Debug("[taskList] Trimming starting and trailing {} characters", "sessionid", uuid)
			config.basicAuthUsername, _ = strings.CutSuffix(config.basicAuthUsername, "}")
			config.basicAuthUsername, _ = strings.CutPrefix(config.basicAuthUsername, "{")
		}

		// Check if input contains any of the configured splitCharacters between }{, like username}@{domain
		slog.Debug("[taskList] Checking input if username is split by any characters", "username", config.basicAuthUsername, "splitCharacters", config.splitCharacters, "sessionid", uuid)
		isComplexInput, inputs := splitComplexInput(config.basicAuthUsername, config.splitCharacters, uuid)
		if isComplexInput {
			slog.Debug("[taskList] Check if input was received from Safeguard", "key", inputs[0], "sessionid", uuid)
			if launcherStdin[inputs[0]] == nil {
				slog.Error("[taskList] Object does not exist in STDIN", "object", inputs[0], "sessionid", uuid)
				os.Exit(1)
			}
			slog.Debug("[taskList] Check if input was received from Safeguard", "key", inputs[2], "sessionid", uuid)
			if launcherStdin[inputs[2]] == nil {
				slog.Error("[taskList] Object does not exist in STDIN", "object", inputs[2], "sessionid", uuid)
				os.Exit(1)
			}
			basicAuthUsername = fmt.Sprint(launcherStdin[inputs[0]]) + inputs[1] + fmt.Sprint(launcherStdin[inputs[2]])

		} else {
			slog.Debug("[taskList] Check if input received from Safeguard", "key", config.basicAuthUsername, "sessionid", uuid)
			if launcherStdin[config.basicAuthUsername] == nil {
				slog.Error("[taskList] Object does not exist in STDIN", "object", config.basicAuthUsername, "sessionid", uuid)
				os.Exit(1)
			}
			basicAuthUsername = fmt.Sprint(launcherStdin[config.basicAuthUsername])

		}
		if strings.Contains(config.url, "https://") {
			config.url, _ = strings.CutPrefix(config.url, "https://")
		}
		if strings.Contains(config.url, "http://") {
			config.url, _ = strings.CutPrefix(config.url, "http://")
		}
		url := "https://" + basicAuthUsername + ":" + fmt.Sprint(launcherStdin["password"]) + "@" + config.url
		urlToLog := "https://" + basicAuthUsername + ":" + "<hidden>" + "@" + config.url
		taskList = append(taskList, chromedp.Action(chromedp.Navigate(url)))
		slog.Debug("[taskList] Navigate to target", "url", urlToLog, "sessionid", uuid)
	} else {
		// Building chromedp taskList from loginActions
		actions := strings.Split(config.loginActions, "||")
		slog.Debug("Parsed "+strconv.Itoa(len(actions))+" actions", "sessionid", uuid)
		slog.Debug("Building chromedp taskList from loginActions..", "sessionid", uuid)

		// Build tasklist
		taskList = append(taskList, chromedp.Action(chromedp.Navigate(config.url)))
		slog.Debug("[taskList] Navigate to target", "url", config.url, "sessionid", uuid)
		for i := 0; i < len(actions); i++ {

			action := strings.Split(actions[i], "::")

			// If browserInputDelay is configured let's pause till that get passed
			if !(config.browserInputDelay == 0) {
				taskList = append(taskList, chromedp.Sleep(time.Millisecond*time.Duration(config.browserInputDelay)))
				slog.Debug("[taskList] Sleep", "sleep_ms", strconv.Itoa(config.browserInputDelay), "sessionid", uuid)
			} else {
				// Otherwise let's wait until the browser presents the element
				taskList = append(taskList, chromedp.WaitReady(action[1]))
				//taskList = append(taskList, chromedp.WaitVisible("body"))
				slog.Debug("[taskList] Waiting element to be visible: "+action[1], "sessionid", uuid)
			}

			keyBoardKey := false
			keyBoardString := false
			if len(action) >= 3 {
				// Is input from Safeguard or a static value?
				if strings.HasPrefix(action[2], "{") && strings.HasSuffix(action[2], "}") {
					// Trim key names from loginActions so that we can use them as keys for the values received from Safeguard via STDIN
					action[2], _ = strings.CutSuffix(action[2], "}")
					action[2], _ = strings.CutPrefix(action[2], "{")
				} else if strings.HasPrefix(action[2], "kb") {
					// Entry will be a keyboard key, not a string or a value from Safeguard
					keyBoardKey = true
				} else {
					// Entry will be a static string from configuration, not a keyboard key or a value from Safeguard
					keyBoardString = true
				}

			}

			switch {
			case action[0] == "c":
				if len(action) == 2 {
					taskList = append(taskList, chromedp.Click(action[1], chromedp.ByID, chromedp.NodeVisible))
					slog.Debug("[taskList] Click", "selector", action[1], "sessionid", uuid)
				} else {
					slog.Error("[taskList] Click action with improper number of configuration items. Format: c::<selector>", "action", actions[i], "sessionid", uuid)
					os.Exit(1)
				}
			case action[0] == "v":
				if len(action) == 3 {
					// Enter value from Safeguard
					if !keyBoardKey && !keyBoardString {
						// Check if input contains any of the configured splitCharacters between }{, like username}@{domain
						slog.Debug("[taskList] Checking input if key is split by any characters", "input", action[2], "splitCharacters", config.splitCharacters, "sessionid", uuid)
						isComplexInput, inputs := splitComplexInput(action[2], config.splitCharacters, uuid)
						if isComplexInput {
							slog.Debug("[taskList] Check if input was received from Safeguard", "key", inputs[0], "sessionid", uuid)
							if launcherStdin[inputs[0]] == nil {
								slog.Error("[taskList] Object does not exist in STDIN", "object", inputs[0], "sessionid", uuid)
								os.Exit(1)
							}
							if launcherStdin[inputs[2]] == nil {
								slog.Debug("[taskList] Check if input was received from Safeguard", "key", inputs[2], "sessionid", uuid)
								slog.Error("[taskList] Object does not exist in STDIN", "object", inputs[2], "sessionid", uuid)
								os.Exit(1)
							}
							val := fmt.Sprint(launcherStdin[inputs[0]]) + inputs[1] + fmt.Sprint(launcherStdin[inputs[2]])
							taskList = append(taskList, chromedp.SendKeys(action[1], val, chromedp.ByID, chromedp.NodeVisible))
							slog.Debug("[taskList] Enter value", "selector", action[1], "value", val, "sessionid", uuid)

						} else {
							slog.Debug("[taskList] Check if input received from Safeguard", "key", action[2], "sessionid", uuid)
							if launcherStdin[action[2]] == nil {
								slog.Error("[taskList] Object does not exist in STDIN", "object", action[2], "sessionid", uuid)
								os.Exit(1)
							}
							taskList = append(taskList, chromedp.SendKeys(action[1], fmt.Sprint(launcherStdin[action[2]]), chromedp.ByID, chromedp.NodeVisible))
							slog.Debug("[taskList] Enter value", "selector", action[1], "value", fmt.Sprint(launcherStdin[action[2]]), "sessionid", uuid)
						}
					} else if keyBoardString {
						// Enter static string from configuration
						taskList = append(taskList, chromedp.SendKeys(action[1], fmt.Sprint(action[2]), chromedp.ByID, chromedp.NodeVisible))
						slog.Debug("[taskList] Enter value", "selector", action[1], "value", fmt.Sprint(action[2]), "sessionid", uuid)
					} else if keyBoardKey {
						// Enter static keyboard key from configuration
						switch {
						case action[2] == "kb.Enter":
							taskList = append(taskList, chromedp.SendKeys(action[1], kb.Enter, chromedp.ByID, chromedp.NodeVisible))
							slog.Debug("[taskList] Enter keyboard key", "selector", action[1], "key", action[2], "sessionid", uuid)
						default:
							slog.Error("[taskList] Key not supported", "key", action[2], "sessionid", uuid)
							os.Exit(1)
						}
					}
				} else {
					slog.Error("[taskList] Enter value action with improper number of configuration items. Format: v::<selector>::<value>", "action", actions[i], "sessionid", uuid)
					os.Exit(1)
				}
			case action[0] == "s":
				if len(action) == 3 {
					if launcherStdin[action[2]] == nil {
						slog.Error("[taskList] Object does not exist in STDIN", "object", action[2], "sessionid", uuid)
						os.Exit(1)
					}

					taskList = append(taskList, chromedp.SendKeys(action[1], fmt.Sprint(launcherStdin[action[2]]), chromedp.ByID, chromedp.NodeVisible))
					slog.Debug("[taskList] Enter secret", "selector", action[1], "value", "<hidden>", "sessionid", uuid)
				} else {
					slog.Error("[taskList] Enter secret action with improper number of configuration items. Format: s::<selector>::<secret-value>", "action", actions[i], "sessionid", uuid)
					os.Exit(1)
				}
			case action[0] == "o":
				if launcherStdin[action[2]] == nil {
					slog.Error("Object does not exist in STDIN", "object", action[2], "sessionid", uuid)
					os.Exit(1)
				}

				t := fmt.Sprint(launcherStdin[action[2]])

				minTimeBeforeExpiry := 0
				if len(action) != 4 && len(action) != 3 {
					slog.Error("[taskList] Enter TOTP code action with improper number of configuration items. Format: o::<selector>::<totp-info-from-safeguard>::<optional--min-seconds-before-expiry>", "action", actions[i], "sessionid", uuid)
					os.Exit(1)
				} else if len(action) == 4 {
					minTimeBeforeExpiry, err = strconv.Atoi(action[3])
				}

				slog.Debug("[taskList] Looking up valid TOTP code...", "sessionid", uuid)
				slog.Debug("[taskList][TOTP_Lookup] Required seconds before TOTP expiry: "+strconv.Itoa(minTimeBeforeExpiry), "sessionid", uuid)
				slog.Debug("[taskList][TOTP_Lookup] TOTP JSON: "+t, "sessionid", uuid)

				var otps []map[string]interface{}

				if len(t) > 0 {
					if err = json.Unmarshal([]byte(t), &otps); err != nil {
						slog.Error("[taskList][TOTP_Lookup] Error occured while parsing TOTP JSON", "sessionid", uuid)
						slog.Error(err.Error(), "sessionid", uuid)
						os.Exit(1)
					}

					otp := ""

					for o := 0; o < len(otps); o++ {
						currentUnixTime := time.Now().Unix()

						totp_UnixTime := fmt.Sprintf("%.0f", otps[o]["UnixTime"])
						totp_Period := fmt.Sprintf("%.0f", otps[o]["Period"])

						slog.Debug("[taskList][TOTP_Lookup] Current UnixTime: "+strconv.Itoa(int(currentUnixTime)), "sessionid", uuid)
						slog.Debug("[taskList][TOTP_Lookup] Examining TOTP "+strconv.Itoa(o+1), "UnixTime", totp_UnixTime, "sessionid", uuid)
						slog.Debug("[taskList][TOTP_Lookup] Examining TOTP "+strconv.Itoa(o+1), "Period", totp_Period, "sessionid", uuid)

						// Check whether the validity of the current TOTP code is within the defined period
						// (current time + the minimum number of seconds required to enter the OTP before it expires)

						// Time until expiry of current code
						totp_UnixTimeInt, err := strconv.Atoi(totp_UnixTime)
						if err != nil {
							slog.Debug("Failed converting Unixtime string to int")
						}
						totp_PeriodInt, err := strconv.Atoi(totp_Period)
						if err != nil {
							slog.Debug("Failed converting Period string to int")
						}
						totp_diff := totp_UnixTimeInt + totp_PeriodInt - int(currentUnixTime)
						if totp_diff >= minTimeBeforeExpiry {
							otp = otps[o]["Code"].(string)
							slog.Debug("[taskList][TOTP_Lookup] Found valid TOTP code, expiring in "+strconv.Itoa(totp_diff)+" seconds", "TOTP_code", otp, "sessionid", uuid)
							o = len(otps)
						} else if totp_diff < 0 {
							slog.Error("[taskList][TOTP_Lookup] TOTP code is already expired. Diff: "+strconv.Itoa(totp_diff)+". Checking the next code", "sessionid", uuid)
						} else {
							slog.Debug("[taskList][TOTP_Lookup] TOTP code is closer to expiry than defined minimum "+strconv.Itoa(minTimeBeforeExpiry)+" seconds. Diff: "+strconv.Itoa(totp_diff)+". Waiting "+strconv.Itoa(minTimeBeforeExpiry)+" seconds before checking the next code.", "sessionid", uuid)
						}

					}
					if otp == "" {
						log.Fatalln("[taskList][TOTP_Lookup] Have not found valid TOTP code, exit.", "sessionid", uuid)
					} else {
						taskList = append(taskList, chromedp.SendKeys(action[1], otp, chromedp.ByID, chromedp.NodeVisible))
						slog.Debug("[taskList] Enter TOTP code", "selector", action[1], "code", otp, "sessionid", uuid)
					}
				}

			}

		}
	}

	// Running task list (built of login actions)
	slog.Debug("Execute taskList", "sessionid", uuid)
	cerr := chromedp.Run(runCtx, taskList...)
	if cerr != nil {
		slog.Error("Error occured while executing taskList", "sessionid", uuid)
		slog.Error("Error: "+cerr.Error(), "sessionid", uuid)
		os.Exit(1)
	}

	os.Exit(0)

}

// If }splitCharacter{ is found in the value input, it returns true, and the splitted strings in array together with the found split character. Otherwise it returns false
func splitComplexInput(input string, splitChars string, uuid string) (bool, []string) {
	var inputs []string
	for i, c := range splitChars {
		slog.Debug("[splitComplexInput] Checking split character: "+fmt.Sprint(i+1), "character", fmt.Sprint(string(c)), "sessionid", uuid)
		match, _ := regexp.MatchString(("}" + "\\" + string(c) + "{"), input)
		if match {
			inputs = strings.Split(input, ("}" + string(c) + "{"))
			inputs = append(inputs, inputs[1])
			inputs[1] = fmt.Sprint(string(c))
			slog.Debug("[splitComplexInput] Match. Returned split string", "#1", inputs[0], "#2", inputs[1], "#3", inputs[2], "sessionid", uuid)
			return true, inputs
		}
	}
	slog.Debug("[splitComplexInput] No any split characters found in key", "key", input, "splitCharacters", splitChars, "sessionid", uuid)
	return false, inputs
}
