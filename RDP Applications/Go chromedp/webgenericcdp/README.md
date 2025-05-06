# Web Generic based on Go ChromeDP

This solution inherits the login flow configuration syntax of [RDP Applications/AutoIt/web_generic](https://github.com/OneIdentity/SafeguardAutomation/tree/master/RDP%20Applications/AutoIt/web_generic)

It supports TOTP code injection.

Logs are written to AppData like web_generic did.
Sample usage: _--cmd "C:\_RApp\webgenericcdp.exe" --args "-url https://10.10.35.170:4433/SGwebtest.html -login v::#account::{account}||s::#password::{password}||v::#targetaccount::{Target.AccountName}||o::#totp::{Target.TotpCodes}::4 -debug"_
 
Other args accepted by webgenericcdp (not tested): 
-edge
-incognito
-delay 1000 (default is 500, delay between login actions)
-insecure (ignore certificate errors)
-debugcdp (enable debug logging for chromedp too)
