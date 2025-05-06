// Author: Viktor Varga (One Identity) reusing some code of Ferenc Sipos (One Identity) and Richard Hosgood (One Identity).

package main

// Importing packages needed by the program
import (
	// Standard library packages
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	// Driver to talk to Chrome-based browsers leveraging the
	// Chrome DevTools protocol
	"github.com/chromedp/chromedp"
)

func main() {

	// Command line parameters
	useEdge := flag.Bool("edge", false, "use MS Edge instead of Chrome")
	useIncognito := flag.Bool("incognito", false, "use incognito mode")
	browserInputDelay := flag.Int("delay", 500, "ms to wait before inputs are submitted")

	//account := flag.String("account", "", "account")
	//password := flag.String("password", "", "password")

	//accountSelector := flag.String("account-selector", "", "account form field CSS selector")
	//passwordSelector := flag.String("password-selector", "", "password form field CSS selector")
	//submitButtonSelector := flag.String("submit-selector", "", "submit form button CSS selector")

	loginActions := flag.String("login", "", "login actions")

	loginUrl := flag.String("url", "", "login URL")
	ignoreCertificateErrors := flag.Bool("insecure", false, "skip certificate validation")

	debug := flag.Bool("debug", false, "enable debug logging")
	debugcdp := flag.Bool("debugcdp", false, "enable debug logging for chromedp")

	flag.Parse()

	// Initialize log file
	userProfileDir := os.Getenv("USERPROFILE")
	f, err := os.OpenFile(userProfileDir+"\\AppData\\Roaming\\OneIdentity\\OI-SG-RemoteApp-Launcher-Orchestration\\webgenericcdp_"+time.Now().Format(time.DateOnly)+".log", os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		//panic(err)
		fmt.Println(err)
		fmt.Println("Cannot create or open log file. Press any key to exit...")
		_, err := fmt.Scanf("%s")
		if err != nil {
			defer f.Close()
			os.Exit(1)
		}
	}
	defer f.Close()

	uuid := uuid.New().String()
	var logLevel = new(slog.LevelVar)
	logger := slog.NewTextHandler(f, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(logger))

	slog.Info("Starting webgenericcdp..", "sessionid", uuid)
	if *debug {
		logLevel.Set(slog.LevelDebug)
		slog.Debug("Loglevel set to Debug", "sessionid", uuid)
	} else {
		logLevel.Set(slog.LevelInfo)
	}

	slog.Debug("Validating arguments", "sessionid", uuid)
	/*if *account == "" {
		log.Fatalln("Account is missing")
	}

	if *password == "" {
		log.Fatalln("Password is missing")
	}*/

	if *loginUrl == "" {
		slog.Error("Login URL is missing", "sessionid", uuid)
		os.Exit(4)
	}

	/*if *accountSelector == "" || *passwordSelector == "" || *submitButtonSelector == "" {
		log.Fatalln("All login form field and button CSS selectors must be specified")
	}*/
	if *loginActions == "" {
		slog.Error("Login actions are missing", "sessionid", uuid)
		os.Exit(4)
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
	if *useEdge {
		edgePath := "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe"
		slog.Debug("Using Edge", "path", edgePath, "sessionid", uuid)
		opts = append(opts,
			chromedp.ExecPath(edgePath),
		)
	}
	if *useIncognito {
		slog.Debug("Using Incognito mode", "sessionid", uuid)
		opts = append(opts,
			chromedp.Flag("incognito", true),
		)
	}
	if *ignoreCertificateErrors {
		slog.Debug("Ignore Certificate Errors", "sessionid", uuid)
		opts = append(opts,
			chromedp.Flag("ignore-certificate-errors", true),
		)
	}

	/*browserOpts := make([]chromedp.ContextOption, 0)
	if *debug {
		browserOpts = append(browserOpts,
			chromedp.WithDebugf(log.Printf),
		)
	}*/

	// Create context with the options defined above
	//allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	//defer cancel() // Free resources when finished

	// Add Debug log option if debug is true
	browserOpts := []chromedp.ContextOption{chromedp.WithLogf(slog.Debug)}
	if *debugcdp {
		slog.Debug("Adding debug log option to chromedp", "sessionid", uuid)
		browserOpts = append(browserOpts, chromedp.WithDebugf(slog.Debug))
	}

	// Create a new execution context
	//runCtx, cancel := chromedp.NewContext(allocCtx, browserOpts...)
	runCtx, _ := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	//defer cancel() // Free resources when finished

	// Parse login actions and build task list
	/*re := regexp.MustCompile("||s::.*::.*||")
	obfsLoginActions := re.ReplaceAll(*loginActions, "||s::<secret::.*||")*/
	//slog.Debug("Parsing login actions", "login", *loginActions)
	actions := strings.Split(*loginActions, "||")
	slog.Debug("Parsed "+strconv.Itoa(len(actions))+" actions", "sessionid", uuid)
	slog.Debug("Building chromedp taskList..", "sessionid", uuid)

	// Build tasklist
	taskList := []chromedp.Action{chromedp.Navigate(*loginUrl)}
	slog.Debug("[taskList] Navigate to target", "url", *loginUrl, "sessionid", uuid)
	for i := 0; i < len(actions); i++ {

		taskList = append(taskList, chromedp.Sleep(time.Millisecond*time.Duration(*browserInputDelay)))
		slog.Debug("[taskList] Append sleep: "+strconv.Itoa(*browserInputDelay)+" ms", "sessionid", uuid)

		action := strings.Split(actions[i], "::")
		switch {
		case action[0] == "c":
			if len(action) == 2 {
				taskList = append(taskList, chromedp.Click(action[1], chromedp.ByID, chromedp.NodeVisible))
				slog.Debug("[taskList] Append action: Click", "selector", action[1], "sessionid", uuid)
			} else {
				slog.Error("[taskList] Click action with improper number of configuration items. Format: c::<selector>", "action", actions[i], "sessionid", uuid)
				os.Exit(1)
			}
		case action[0] == "v":
			if len(action) == 3 {
				taskList = append(taskList, chromedp.SendKeys(action[1], action[2], chromedp.ByID, chromedp.NodeVisible))
				slog.Debug("[taskList] Append action: Enter value", "selector", action[1], "value", action[2], "sessionid", uuid)
			} else {
				slog.Error("[taskList] Enter value action with improper number of configuration items. Format: v::<selector>::<value>", "action", actions[i], "sessionid", uuid)
				os.Exit(1)
			}
		case action[0] == "s":
			if len(action) == 3 {
				taskList = append(taskList, chromedp.SendKeys(action[1], action[2], chromedp.ByID, chromedp.NodeVisible))
				slog.Debug("[taskList] Append action: Enter secret", "selector", action[1], "sessionid", uuid)
			} else {
				slog.Error("[taskList] Enter secret action with improper number of configuration items. Format: s::<selector>::<secret-value>", "action", actions[i], "sessionid", uuid)
				os.Exit(1)
			}
		case action[0] == "o":
			t := action[2]

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

					log.Fatalln(err)
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
					slog.Debug("[taskList] Append action: Enter TOTP code", "selector", action[1], "code", otp, "sessionid", uuid)
				}
			}

		}

	}

	// Running task list (built of login actions)
	cerr := chromedp.Run(runCtx, taskList...)
	if cerr != nil {
		log.Fatalln(cerr)
		os.Exit(1)
	}

}
