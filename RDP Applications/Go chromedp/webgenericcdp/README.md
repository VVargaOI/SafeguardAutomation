# Webgenericcdp - Generic web automation script for Safeguard based on Go/chromedb

Webgenericcdp works only with OI-SG-RemoteApp-Launcher version 3.0.1 or later, together with the --use-stdin parameter/ This restriction ensures that no sensitive information is visible in command line parameters.

## App publishing

When the --use-stdin parameter is used, the value of the --args parameter is also passed by the RemoteApp-Launcher to the application set in --cmd.

Webgenericcdp expects the path of its configuration file given in --args, optionally with the -debug switch.

Sample RemoteApp publishing parameter configuration:

```--use-stdin --args "C:\_RApp\webgenericcdp_Entra_with_MFA_incognito.conf -debug" --cmd "c:\_RApp\webgenericcdp.exe"```

## Configuring webgenericcdp

Sample configuration file including the configuration parameters' description is shared together with webgenericcdp.

Settings which are uncommented or shown without a default value in the sample confgiuration are mandatory.

The ```loginActions``` setting is backwards compatible with the syntax used at [AutoIt/web_generic](https://github.com/OneIdentity/SafeguardAutomation/tree/master/RDP%20Applications/AutoIt/web_generic)

The ```loginActions``` setting supports concatenated input values, like UPN or down-level logon names, which is built of multiple key-value pairs received from Safeguard.

Webgenericcdp by default waits for the next element ```loginActions``` being loaded by the browser, however it is not reliable on all websites. To overcome that, ```browserInputDelay``` can be configured which pauses the execution before performing the next action.

If ```basicAuthUsername``` is configured, ```loginActions``` is ignored and the configured web application is accessed using basic authentication.

## Troubleshooting

Logs are written into the following folder of the RDP host account: %AppData%\OneIdentity\OI-SG-RemoteApp-Launcher-Orchestration

Debug logging for webgenericcdp can be enabled via the app publishing configuration using the -debug switch within --args.

Debug logging for chromedp can be enabled the ```chromedb_logging``` setting within the configuration file.

If the RemoteApp-Launcher console window does not close it means that the script did not find all elements on the page and it's still retrying.

### Other issues

* In case of running lots of tests within a short period of time, Windows Security may flag webgenericcdp as Trojan. Symptom is that the RemoteApp-Launcher window closes and there is no any log from webgenericcdp. The following event is visible in the OI-SG-RemoteApp-Launcher log: *Error occurred while trying to launch application: Error occurred while trying to launch process. Got error: Operation did not complete successfully because the file contains a virus or potentially unwanted software. (os error 225)*
FIXME:picture
In that case Allow it via Windows Security
FIXME:picture


## Known issues

* ```browser_incognito=true``` does not work when using Edge
* ```chromedp_logging=info``` does not work




