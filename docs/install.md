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

Use `fj` (tested with 0.7.0) if `tea` on your `PATH` belongs to something else, such as the
LocalGit `tea` shim. Each remote then goes to the CLI that has a login for it:

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

## Not covered yet

- Running at logon or as a Windows service. The binary supports running as a Windows service,
  but this setup hasn't been tried.
- Backups. Stop the server and copy its folder, or use `gitea dump`.
- Web links. API responses still contain `html_url` links, but there is no web UI to open them.
