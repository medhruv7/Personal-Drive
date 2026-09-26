# Local Drive

A personal "Google Drive" served from your own computer. Point it at a folder, and you can browse, upload, download, and create folders from any browser: laptop or phone.

- Single binary with the web UI built in. No dependencies, no database.
- Runs on macOS, Windows, and Linux.
- Uploads stream straight to disk, so multi-GB files work. You get drag-and-drop, multiple files at once, and progress bars.
- Downloads support resume and video seeking. Folders download as a zip.
- Uploading a file with an existing name never overwrites it. It gets saved as `name (1).ext` instead.
- File access is sandboxed to the chosen folder with Go's `os.Root`, so `../` tricks and symlinks can't reach anything outside it.

## Run it

With Go installed (1.26+):

```sh
go run .                          # serves ~/LocalDrive (created if missing)
go run . -root "D:\Files"         # Windows: serve a specific folder
go run . -root ~/Drive -addr :9000
```

Or use a prebuilt binary (see **Build**):

```sh
./localdrive-darwin-arm64 -root ~/Drive          # macOS (Apple Silicon)
localdrive-windows-amd64.exe -root "D:\Files"    # Windows
```

On startup it prints the URLs to open:

```
  On this computer:  http://localhost:8080
  On your network:   http://192.168.1.2:8080
```

### Options

| Flag | Env var | Default | Meaning |
|---|---|---|---|
| `-root` | `LOCALDRIVE_ROOT` | `~/LocalDrive` | Folder to serve |
| `-addr` | `LOCALDRIVE_ADDR` | `:8080` | Listen address (`:8080` = all interfaces) |
| `-show-hidden` | `LOCALDRIVE_SHOW_HIDDEN` | `false` | Show dotfiles, `desktop.ini`, `Thumbs.db` |
| `-max-upload` | `LOCALDRIVE_MAX_UPLOAD` | `0` (unlimited) | Max bytes per upload request |

## Use it from your phone

1. Connect the phone to the same Wi-Fi as the computer.
2. Open the `On your network` URL printed at startup.
3. The first time, your OS may ask to allow incoming connections:
   - **macOS:** click *Allow* on the firewall prompt.
   - **Windows:** tick *Private networks* on the Defender Firewall prompt and click *Allow access*.

Tip: on iOS or Android use *Add to Home Screen* to get an app-like icon.

## Build

```sh
./build.sh      # → dist/localdrive-{darwin,windows,linux}-{amd64,arm64}[.exe]
```

To build just for the machine you're on, run `go build -o localdrive .` (on Windows, use `-o localdrive.exe`).

## Test

```sh
go test ./...
```

## API

All paths are forward-slash paths relative to the drive root, e.g. `/Photos/2024`.

| Method | Endpoint | Notes |
|---|---|---|
| GET | `/api/list?path=/x` | `{path, entries:[{name,isDir,size,modTime}]}` |
| GET | `/api/download?path=/x/file` | Supports `Range` |
| GET | `/api/zip?path=/x` | Streams the folder as `x.zip` |
| POST | `/api/upload?path=/x` | `multipart/form-data`, one or more `files` fields |
| POST | `/api/mkdir` | JSON `{"path": "/x", "name": "New"}` |

Example: `curl -F files=@photo.jpg "http://localhost:8080/api/upload?path=/Photos"`

## Hosting with Tailscale (recommended)

To reach the drive from your phone anywhere, privately and over HTTPS, without opening it to the internet, follow **[SETUP.md](SETUP.md)**.

## Before exposing it to the internet

**There is no authentication.** Anyone who can reach the port can read and upload files. That's fine on your home Wi-Fi, but not on a public address. Before hosting it:

1. **Put it behind HTTPS.** [Caddy](https://caddyserver.com) makes this a few lines, and gets certificates automatically:
   ```
   drive.example.com {
       basic_auth {
           you $2a$14$...   # generate with: caddy hash-password
       }
       reverse_proxy localhost:8080
   }
   ```
2. **Add authentication.** Either use the proxy's basic auth shown above, or add a login to the app itself. The middleware chain in `Server.Handler` (`server.go`) is the place for it.
3. When a proxy sits in front, bind Local Drive to localhost only (`-addr 127.0.0.1:8080`) so it can't be reached directly.

## Layout

```
main.go         flags, startup banner, HTTP server
server.go       routes and handlers (list, download, zip, upload, mkdir)
paths.go        path resolution and filename validation
server_test.go  tests, including path-traversal and symlink-escape cases
web/            the UI (HTML/CSS/JS), embedded into the binary
build.sh        cross-compile script
```
