# runas

`runas` launches a Windows application under a different user account using
credentials supplied by One Identity Safeguard for Privileged Passwords (SPP)
via STDIN. It is designed to be invoked by **OI-RemoteDesktopLauncher 3.0.1+**
as the launch "cmd" for a Remote Application session.

Credentials are passed directly to the Win32 `CreateProcessWithLogonW` API
(`advapi32.dll`) and are **never** written to disk, environment variables, or
the child process's command line — they are only ever visible on the Launcher
STDIN pipe and in this process's memory for the duration of the call.

## How it works

1. `OI-RemoteDesktopLauncher` starts `runas.exe` and writes a single line of
   JSON to its STDIN containing the launch context (target account, domain,
   password, asset address, etc.) and the CLI arguments configured for the
   launcher.
2. `runas` reads that JSON, parses the `cli_args` field to find its config
   file path (and an optional `-debug` flag).
3. It reads the `.conf` file to determine which application to launch
   (`appPath`) and what arguments to pass (`appArgs`, with `{StdinKey}`
   placeholders resolved from the STDIN JSON).
4. It calls `CreateProcessWithLogonW` to start that application as the
   target Windows user, then exits without waiting for the child process.

## Usage

Configure the Remote Application in `OI-RemoteDesktopLauncher` with:

```
--use-stdin --args "<full-path>\runas.conf -debug" --cmd "<full-path>\runas.exe"
```

- `<full-path>\runas.conf` — path to the config file for this application
  (see [Configuration](#configuration) below).
- `-debug` — optional; enables verbose (`Debug`-level) logging.

## Configuration

Each Remote Application should have its own `.conf` file. Lines starting
with `#` and blank lines are ignored. Keys are `key=value` pairs:

| Key              | Required | Values           | Description |
|------------------|----------|------------------|--------------|
| `mode`           | no (default `p`) | `p` or `e` | `p` = `appPath` is a direct filesystem path to the exe. `e` = `appPath` is the *name* of an environment variable whose value is the exe path. |
| `appPath`        | yes      | path or env var name | The application to launch. |
| `appArgs`        | no       | string           | Arguments passed to the application. Supports `{StdinKey}` placeholders (see below), resolved from the Launcher's STDIN JSON at runtime. |
| `dumpStdinToLog` | no (default `false`) | `true` or `false` | If `true`, writes the raw STDIN JSON (including the password) to the log file. Debug/troubleshooting only — **never enable in production**. |

### `{StdinKey}` placeholders

`appArgs` may reference any field present in the Launcher's STDIN JSON using
`{KeyName}` syntax, for example:

- `{Target.AssetNetworkAddress}`
- `{Target.AccountName}`
- `{Target.AccountDomainName}`

**Never** reference `{password}` in `appArgs` — resolved arguments are placed
on the child process's command line, which is visible to other processes via
the Windows API. Credentials must only ever reach the launched application
through means the application itself supports (e.g. Windows single sign-on
via `CreateProcessWithLogonW`), not via `appArgs`.

For security, resolved placeholder values must not contain a double-quote
(`"`) character — this is rejected at runtime to prevent argument injection
into the launched process's command line.

### Sample config files

See `runas_sample.conf` and the examples under `release/runas_v1.0.0/`:

- `runas_sample.conf` — annotated template.
- `runas_SSMS_WindowsAuth.conf` — launches SQL Server Management Studio
  connecting to `{Target.AssetNetworkAddress}` with Windows Authentication.
- `runas_MMC_CertMgr.conf` — launches the Certificates MMC snap-in
  (`certmgr.msc`).

## Logging

Logs are written to:

```
%USERPROFILE%\AppData\Roaming\OneIdentity\OI-SG-RemoteApp-Launcher-Orchestration\runas_<YYYY-MM-DD>.log
```

- One log file per day, appended to on each run.
- Default level is `Info`; pass `-debug` in `cli_args` to enable `Debug`
  level (includes config file contents, resolved paths, and the exact
  command line passed to `CreateProcessWithLogonW`).
- Each log entry includes a per-run `sessionid` (UUID) to correlate lines
  from the same invocation.
- The password itself is only ever logged if `dumpStdinToLog=true` — leave
  this disabled outside of troubleshooting.

## Required STDIN JSON fields

`runas` requires the following fields to be present in the JSON written to
its STDIN by the Launcher:

- `cli_args` — the config file path (and optional ` -debug` suffix).
- `Target.AccountName`
- `Target.AccountDomainName`
- `password`

`Target.AssetNetworkAddress` is optional and used only for logging/args if
referenced.

## Building a binary

`runas` is a standard Go module (`module runas`, `go.mod` at the repo root).
The build version is injected at link time into the `main.version` package
variable (defaults to `"dev"` if not set).

From this directory:

```powershell
go build -ldflags="-X main.version=1.0.0" -o runas.exe .
```

- Replace `1.0.0` with the release version being built.
- Omit `-ldflags` for a local/dev build (`version` will report `"dev"`).

To verify the embedded version of a built binary:

```powershell
go version -m .\runas.exe | Select-String -Pattern "ldflags|main.version"
```

Dependencies (currently just `github.com/google/uuid`) are managed via
`go.mod`/`go.sum`; run `go mod tidy` after adding or changing imports.
