# sqlstudio

Launches SQL Server Management Studio (SSMS) 22 or later with Entra ID Interactive authentication (supporting login with password and OTP provided by SPP) and automates the login dialog using Windows UI Automation (UIA). Designed to run as a RemoteApp under **OI-RemoteDesktopLauncher 3.0.1+**.

It starts SSMS with 'ssms.exe -S {Target.AssetNetworkAddress} -U {Target.AccountName@Target.AccountDomainName} -A ActiveDirectoryInteractive'

Values are received via STDIN from the Launcher.

## Launcher configuration

```
--use-stdin --args "<full-path>\sqlstudio.conf -debug" --cmd "<full-path>\sqlstudio.exe"
```

## sqlstudio.conf

| Key | Required | Default | Description |
|-----|----------|---------|-------------|
| `ssmspath` | Yes | — | Directory containing `ssms.exe` |
| `encrypt` | No | `optional` | Encryption for the SSMS connection: `optional`, `mandatory`, or `strict` (passed as `-N`) |
| `trustServerCertificate` | No | `true` | Trust the server certificate without validation (passed as `-C`) |
| `loginActions` | No | see below | Login automation steps |
| `browserInputDelay` | No | `1000` | Delay in ms after each click action |
| `uiaWaitTimeout` | No | `60` | Max seconds to wait for each UIA element |
| `dumpStdinToLog` | No | `false` | Log full STDIN JSON (debug only — logs credentials) |

### Default loginActions

```
s::#i0118::{password}||c::#idSIButton9::10||o::#idTxtBx_SAOTCC_OTC::{Target.TotpCodes}::3||c::#idSubmit_SAOTCC_Continue::10||c::#idBtn_Back::5
```

Steps: enter password → click Sign in → enter TOTP → click Submit → click "No, this app only".

### loginActions syntax

Actions are `||`-separated. Each action is `type::selector[::value[::param]]`.

| Type | Syntax | Description |
|------|--------|-------------|
| `s` | `s::#AID::{StdinKey}` | Set element value from STDIN (secret, delivered via pipe) |
| `v` | `v::#AID::{StdinKey}` | Same as `s` |
| `c` | `c::#AID[::N]` | Click element (optional — skipped if not found; waits N seconds, default 10) |
| `o` | `o::#AID::{Target.TotpCodes}::N` | Enter TOTP code with at least N seconds before expiry |

The `#` prefix on selectors is stripped; the remainder is matched against the UIA `AutomationId` property.

## Security

Credentials are never stored in environment variables or temporary files. The PowerShell automation script is passed as a base64-encoded command (no secrets). Secrets are delivered as a single `base64(JSON)` line written to the process's anonymous stdin pipe, readable only by the subprocess.

## Build

```powershell
go build -o sqlstudio.exe .
```

## Log location

```
%USERPROFILE%\AppData\Roaming\OneIdentity\OI-SG-RemoteApp-Launcher-Orchestration\sqlstudio_<date>.log
```
