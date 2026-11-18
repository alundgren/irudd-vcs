# Installing irudd-vcs for t3code

These steps set up irudd-vcs on a Windows PC as the Gitea/Forgejo backend for t3code.
The commands are for PowerShell. Replace `alice` and the paths with your own.

## 1. Build the binary

Build with the `bindata` tag. Without it, the binary reads its readme, license and gitignore
templates from the source tree, and repository creation fails once you copy the binary
elsewhere (the server then logs `No readme templates found` at startup).

```powershell
cd M:\repos\irudd-vcs
Push-Location modules/options; go run ../../build/generate-bindata.go ../../options bindata.dat; Pop-Location
$env:CGO_ENABLED = "0"
$version = (git describe --tags --always) -replace '^v', '' -replace '^([^-]+)-', '$1+'
go build -trimpath -tags bindata -ldflags "-s -w -X main.Version=$version -X main.Tags=bindata" -o gitea.exe .
```

The version must be a semantic version such as `1.27.3+125-gf8b3409315`, so the build needs the
`v1.27.3` tag on the upstream commit the fork starts from. Without it the server reports a bare
commit hash, and `tea` and the Gitea SDK treat it as Gitea 1.11 and turn off newer features.

With make (Linux, macOS or WSL), `TAGS=bindata make backend` does the same.

## 2. Install and start

The binary keeps its config and data next to itself, so give it a folder of its own:

```powershell
$dir = "$env:LOCALAPPDATA\irudd-vcs"
New-Item -ItemType Directory -Force $dir
Copy-Item gitea.exe $dir
& "$dir\gitea.exe" web
```

The first start writes `custom\conf\app.ini` with generated secrets and stores everything else
under `data\`. The server listens on all interfaces on port 3000. To change that, stop the
server and add these lines to `app.ini`:

```ini
[server]
HTTP_ADDR = 127.0.0.1
HTTP_PORT = 3000
```

Set the port in `app.ini`, not with `web --port`. Hooks follow `--port` too, but other
commands such as `gitea manager` only read the config.

## 3. Create the owner account

In a second terminal:

```powershell
& "$env:LOCALAPPDATA\irudd-vcs\gitea.exe" admin user create --username alice --email alice@example.com
```

This prints an access token with all scopes, and it is shown only once. Every API and git request
needs a token; there are no passwords. To make more tokens, for example one with fewer
scopes for an agent, use `gitea admin user generate-access-token --help`.

## 4. Log in a client for t3code

t3code talks to Forgejo and Gitea servers through the `fj` or `tea` CLI. It tries `fj` first
for each remote, then `tea`.

Use `fj` (tested with 0.7.0) if `tea` on your `PATH` belongs to another tool.
Each remote then goes to the CLI that has a login for it:

```powershell
cargo install forgejo-cli --version 0.7.0 --locked
"<token>" | fj -H http://localhost:3000 auth add-token
fj -H http://localhost:3000 whoami
```

With the real `tea` 0.16 or later, use:

```powershell
tea login add --name irudd-vcs --url http://localhost:3000 --token <token>
```

`tea` warns that `/api/v1/user/keys` returned 404 because the server has no SSH. You can
ignore it.

## 5. Let git push and clone

Git authenticates over HTTP with the same token as its password. To store it in Git Credential
Manager once, so git doesn't prompt:

```powershell
"protocol=http`nhost=localhost:3000`nusername=alice`npassword=<token>`n" | git credential approve
```

## 6. Check it

```powershell
curl.exe http://localhost:3000/api/v1/version
curl.exe -H "Authorization: token <token>" http://localhost:3000/api/v1/user
git clone http://localhost:3000/alice/<repo>.git
```

Create repositories with `POST /api/v1/user/repos`, or from t3code, then push to them.
A push to a repository that doesn't exist yet fails, unless you set
`[repository] ENABLE_PUSH_CREATE_USER = true` so the push creates it.

## 7. Run it as a Windows service

`gitea.exe web` in a terminal keeps a console window open. As a Windows service it runs from
boot with no window and without anyone logged in, and restarts if it crashes. Run these steps once
in an elevated PowerShell.

Services usually keep their data under `C:\ProgramData`. Stop the server, move its folder there and
point `WORK_PATH` in `custom\conf\app.ini` at the new folder. The service has no console, so also
log to files under `log\`:

```powershell
$dir = "C:\ProgramData\irudd-vcs"
Move-Item "$env:LOCALAPPDATA\irudd-vcs" $dir
```

```ini
WORK_PATH = C:\ProgramData\irudd-vcs

[log]
MODE = file
```

Create the service under the low-privilege LocalService account. Give that account write access to
the folder, and keep full access for your own account so the `gitea admin` commands still work:

```powershell
icacls $dir /grant '*S-1-5-19:(OI)(CI)M' "${env:USERNAME}:(OI)(CI)F" /T /Q
sc.exe create irudd-vcs binPath= "`"$dir\gitea.exe`" web --work-path `"$dir`"" start= delayed-auto obj= 'NT AUTHORITY\LocalService' DisplayName= 'irudd-vcs'
sc.exe failure irudd-vcs reset= 86400 actions= restart/5000/restart/5000/restart/60000
sc.exe start irudd-vcs
```

To stop and start the service from your own account without elevation, for example to update
the binary, add your account to the service's permissions:

```powershell
$me = ([Security.Principal.WindowsIdentity]::GetCurrent()).User.Value
$sd = (sc.exe sdshow irudd-vcs | Where-Object { $_ }) -join ''
sc.exe sdset irudd-vcs ($sd -replace '^(D:)', "`$1(A;;RPWPLCLORC;;;$me)")
```

To update the binary, stop the service and wait for its process to exit before you copy. The
process keeps the file locked for a moment after the service reports that it stopped:

```powershell
$id = (Get-CimInstance Win32_Service -Filter "Name='irudd-vcs'").ProcessId
Stop-Service irudd-vcs
while (Get-Process -Id $id -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 250 }
Copy-Item gitea.exe "$dir\gitea.exe" -Force
Start-Service irudd-vcs
```

## 8. Reach it from other machines over HTTPS

The server can keep listening on `127.0.0.1` only. To reach it from other machines, put an HTTPS
proxy in front of it. With Tailscale, `tailscale serve` uses a certificate for the machine's
MagicDNS name, once HTTPS certificates are enabled for the tailnet. Any free port works; this
example uses 8443:

```powershell
tailscale serve --bg --https=8443 http://127.0.0.1:3000
```

`--bg` saves the setting in Tailscale, so it survives restarts. On Windows, changing the serve
config needs an elevated terminal while any serve entry shares a folder.

Then set the public address in `app.ini`, so clone URLs and links use it, while the hooks keep
using the local one:

```ini
[server]
ROOT_URL = https://<machine>.<tailnet>.ts.net:8443/
LOCAL_ROOT_URL = http://127.0.0.1:3000/
```

Log clients in with that address instead of `http://localhost:3000`, for example
`tea login add --name irudd-vcs --url https://<machine>.<tailnet>.ts.net:8443 --token <token>`,
and store the git credential with `protocol=https` and `host=<machine>.<tailnet>.ts.net:8443`. With
Git Credential Manager, also run
`git config --global credential.https://<machine>.<tailnet>.ts.net:8443.provider generic`,
so it doesn't try to detect the host type on each request.

## Not covered yet

- Backups. Stop the server and copy its folder, or use `gitea dump`.
- Web links. API responses still contain `html_url` links, but there is no web UI to open them.
