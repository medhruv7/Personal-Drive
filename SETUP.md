# Setting up Local Drive with Tailscale

This guide gets Local Drive running on your laptop so you can reach it from your phone, or any of your devices, from anywhere, privately and over HTTPS.

**How it fits together:**

```
 phone (Tailscale app on)                        laptop
 ┌───────────────┐   encrypted Tailscale    ┌──────────────────────────────┐
 │ browser       │ ───────────────────────▶ │ tailscale serve  (HTTPS)     │
 │ https://…ts.net│      network            │        │                     │
 └───────────────┘                          │        ▼                     │
                                            │ localdrive  127.0.0.1:8080   │
                                            │        │                     │
                                            │        ▼                     │
                                            │ ~/LocalDrive  (your files)   │
                                            └──────────────────────────────┘
```

- Local Drive listens on **127.0.0.1 only**, so nothing on your Wi-Fi, or anywhere else, can reach it directly.
- **Tailscale Serve** is the only way in. It adds HTTPS, and only devices logged into *your* Tailscale account can connect.
- No router port forwarding, no public IP, and no firewall rules are needed.

Time needed: about 15 minutes. Mac and Windows steps are shown side by side, so follow the ones for the laptop that will hold the files.

---

## Step 1: Get the Local Drive program

You need one file: the `localdrive` executable. The prebuilt ones are in `dist/`.

| Laptop | File to use |
|---|---|
| Mac with Apple Silicon (M1/M2/M3/M4) | `dist/localdrive-darwin-arm64` |
| Mac with Intel chip | `dist/localdrive-darwin-amd64` |
| Windows (most PCs) | `dist/localdrive-windows-amd64.exe` |
| Windows on ARM (e.g. Surface Pro X / Snapdragon) | `dist/localdrive-windows-arm64.exe` |

If `dist/` is missing or out of date, rebuild it from the project folder (needs Go 1.26+):

```sh
./build.sh
```

### Mac

Copy the program to a stable place outside OneDrive. Background services on macOS can be blocked from reading cloud-synced folders.

```sh
mkdir -p ~/bin ~/LocalDrive
cp dist/localdrive-darwin-arm64 ~/bin/localdrive      # use -amd64 on an Intel Mac
chmod +x ~/bin/localdrive
```

> If you copied the file from another computer (AirDrop, download) and macOS says it "cannot be opened", run
> `xattr -d com.apple.quarantine ~/bin/localdrive`.

### Windows

Open **PowerShell** and run:

```powershell
New-Item -ItemType Directory -Force C:\LocalDrive, "$env:USERPROFILE\LocalDrive"
Copy-Item .\dist\localdrive-windows-amd64.exe C:\LocalDrive\localdrive.exe
```

> Because this project lives in OneDrive, the `dist\` folder may already be synced to your Windows PC. Copy the `.exe` from there.
>
> If Windows shows **"Windows protected your PC"** the first time, click **More info → Run anyway**. The program isn't code-signed, which is normal for something you built yourself.

### Pick your drive folder

The folder you serve is the **root** of your drive. Everything inside it is visible, and nothing outside it is. The defaults used below are:

- Mac: `~/LocalDrive`
- Windows: `%USERPROFILE%\LocalDrive`

You can use any folder instead, such as an external disk like `/Volumes/MyDisk/Drive` or `D:\Drive`. On a Mac, avoid serving `~/Documents`, `~/Desktop` or `~/Downloads` directly, because macOS privacy protection will block background access to them.

---

## Step 2: Test it locally

### Mac

```sh
~/bin/localdrive -root ~/LocalDrive -addr 127.0.0.1:8080
```

### Windows

```powershell
C:\LocalDrive\localdrive.exe -root "$env:USERPROFILE\LocalDrive" -addr 127.0.0.1:8080
```

Open **http://localhost:8080** in a browser on the laptop. Upload a file, check it appears in the folder, then press **Ctrl+C** to stop.

> `-addr 127.0.0.1:8080` is important. It makes Local Drive listen only to the laptop itself. Tailscale will be the only thing that forwards traffic to it.

---

## Step 3: Install Tailscale on the laptop

1. Download and install Tailscale from **https://tailscale.com/download**.
   - **Mac:** use the standalone download or the Mac App Store version.
   - **Windows:** use the Windows installer.
2. Open Tailscale and **log in** with Google, Microsoft, GitHub, or another provider. This creates your private network (your *tailnet*).
3. Make sure Tailscale is set to **start at login**. It is by default: check the Tailscale menu-bar or tray icon under Settings or Preferences.

### Make sure the `tailscale` command works

Open a new terminal and run:

```sh
tailscale status
```

- **Windows:** this works right away.
- **Mac:** if you get `command not found`, either enable the CLI from the Tailscale menu-bar app's settings, or add this alias to `~/.zshrc`:
  ```sh
  alias tailscale="/Applications/Tailscale.app/Contents/MacOS/Tailscale"
  ```
  Then open a new terminal.

---

## Step 4: Turn on HTTPS for your tailnet (one time)

1. Go to the admin console: **https://login.tailscale.com/admin/dns**
2. Make sure **MagicDNS** is enabled.
3. Under **HTTPS Certificates**, click **Enable HTTPS**.

This gives your laptop a name like `my-laptop.tail1234.ts.net` with a real, trusted certificate. No certificate warnings on your phone.

> The machine name is part of the URL. To make it nicer (e.g. `drive`), go to **Machines**, click your laptop, and choose **… → Edit machine name**.

---

## Step 5: Put Local Drive behind Tailscale Serve

1. Start Local Drive again, as in Step 2, and leave it running.
2. In a **second** terminal, run:
   ```sh
   tailscale serve --bg 8080
   ```

It prints your drive's address, something like:

```
Available within your tailnet:

https://my-laptop.tail1234.ts.net/
|-- proxy http://127.0.0.1:8080
```

- `--bg` makes this permanent. Serve keeps running in the background and comes back after a reboot, so you only run this once.
- If the command asks you to enable HTTPS or Serve, follow the link it prints, then run it again.

Useful commands:

```sh
tailscale serve status    # show the current address and what it points to
tailscale serve reset     # turn Serve off completely
```

---

## Step 6: Set up your phone

1. Install **Tailscale** from the App Store (iPhone) or Google Play (Android).
2. Log in with the **same account** you used on the laptop.
3. Turn the Tailscale toggle **on**. It shows up as a VPN, which is expected.
4. Open the `https://….ts.net` address from Step 5 in Safari or Chrome.
5. Optional: add it to your home screen for an app-like icon.
   - **iPhone:** tap Share, then **Add to Home Screen**.
   - **Android:** tap ⋮, then **Add to Home screen**.

Where downloaded files go:

- **iPhone:** the Files app, under **Downloads**.
- **Android:** the **Downloads** folder.

> **Tip for big uploads from the phone:** keep the browser open and the screen on until the upload finishes. Phones often pause uploads when the browser goes to the background.

Any other laptop or tablet works the same way: install Tailscale, log in, and open the address.

---

## Step 7: Start Local Drive automatically

Right now Local Drive stops when you close the terminal. Set it up to start on login and restart if it crashes.

### Mac (launchd)

1. Create the service file. This command fills in your paths automatically; change `-root` if you serve a different folder.

   ```sh
   mkdir -p ~/Library/LaunchAgents ~/Library/Logs
   cat > ~/Library/LaunchAgents/com.localdrive.plist <<EOF
   <?xml version="1.0" encoding="UTF-8"?>
   <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
   <plist version="1.0">
   <dict>
     <key>Label</key><string>com.localdrive</string>
     <key>ProgramArguments</key>
     <array>
       <string>$HOME/bin/localdrive</string>
       <string>-root</string><string>$HOME/LocalDrive</string>
       <string>-addr</string><string>127.0.0.1:8080</string>
     </array>
     <key>RunAtLoad</key><true/>
     <key>KeepAlive</key><true/>
     <key>StandardOutPath</key><string>$HOME/Library/Logs/localdrive.log</string>
     <key>StandardErrorPath</key><string>$HOME/Library/Logs/localdrive.log</string>
   </dict>
   </plist>
   EOF
   ```

2. Start it:

   ```sh
   launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.localdrive.plist
   ```

3. Check it's running:

   ```sh
   curl -s http://127.0.0.1:8080/api/list?path=/ && echo " ← OK"
   tail -f ~/Library/Logs/localdrive.log      # watch requests (Ctrl+C to stop watching)
   ```

Managing the service:

```sh
launchctl kickstart -k gui/$(id -u)/com.localdrive                       # restart (e.g. after updating)
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.localdrive.plist # stop and disable
```

> If you serve a folder on an external disk, macOS may ask once to let `localdrive` access removable volumes. Click **Allow**.

### Windows (Task Scheduler)

1. Open **PowerShell as Administrator**: right-click the Start button, then **Terminal (Admin)**. Run the following, changing `-root` if needed:

   ```powershell
   $exe  = "C:\LocalDrive\localdrive.exe"
   $args = "-root `"$env:USERPROFILE\LocalDrive`" -addr 127.0.0.1:8080"

   $action    = New-ScheduledTaskAction -Execute $exe -Argument $args
   $trigger   = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
   $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType S4U   # runs hidden, no console window
   $settings  = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
                  -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1)

   Register-ScheduledTask -TaskName "LocalDrive" -Action $action -Trigger $trigger `
                          -Principal $principal -Settings $settings
   Start-ScheduledTask -TaskName "LocalDrive"
   ```

2. Check it's running:

   ```powershell
   Invoke-RestMethod "http://127.0.0.1:8080/api/list?path=/"
   ```

Managing the task:

```powershell
Stop-ScheduledTask  -TaskName "LocalDrive"                  # stop
Start-ScheduledTask -TaskName "LocalDrive"                  # start / restart after updating
Unregister-ScheduledTask -TaskName "LocalDrive" -Confirm:$false   # remove
```

> You can also see and edit the task in the **Task Scheduler** app, under Task Scheduler Library → LocalDrive.

---

## Step 8: Keep the laptop reachable

Local Drive only works while the laptop is **on, awake, and online**.

### Mac

- **System Settings → Battery → Options** (or **Energy Saver** on desktops): turn on **"Prevent automatic sleeping on power adapter when the display is off"**.
- Closing the lid puts a MacBook to sleep, unless it's connected to power *and* an external display. For a quick session you can also run `caffeinate -s` in a terminal, which keeps the Mac awake while plugged in until you press Ctrl+C.

### Windows

- **Settings → System → Power & battery → Screen and sleep**: set **"When plugged in, put my device to sleep after"** to **Never**.
- **Control Panel → Power Options → Choose what closing the lid does**: set **When I close the lid (Plugged in)** to **Do nothing**.

---

## Done: daily use

- **At home or away:** turn on the Tailscale app on your phone and open your bookmark.
- **The laptop side** needs nothing once Steps 5 and 7 are done. Tailscale, Serve, and Local Drive all start automatically on login.

## Updating Local Drive later

1. In the project folder, rebuild: `./build.sh`
2. Copy the new binary over the old one:
   - **Mac:** `cp dist/localdrive-darwin-arm64 ~/bin/localdrive`
   - **Windows:** stop the task first, then copy the new `.exe` to `C:\LocalDrive\localdrive.exe`
3. Restart the service, using the command from Step 7.

## Sharing with someone else (optional)

- **Option 1:** invite them to your tailnet from the admin console (**Users → Invite users**). They can then reach the drive with their own Tailscale login.
- **Option 2:** share just this one laptop from **Machines → your laptop → Share**.

Anyone you give access to gets **full read and upload access** to the drive, since Local Drive itself has no login yet.

> **Don't use `tailscale funnel`** with Local Drive. Funnel publishes the address to the whole internet, and without a login anyone who found the URL could read and upload files.

---

## Troubleshooting

| Problem | Fix |
|---|---|
| Phone can't open the `ts.net` address | Is the Tailscale toggle on in the phone app? Is the laptop awake? Does `tailscale status` on the laptop list the phone? |
| Browser shows **502 Bad Gateway** | Serve is working but Local Drive isn't running. Start it, or check the service from Step 7. |
| `tailscale serve` says HTTPS is not enabled | Do Step 4, or follow the link the command prints. |
| `tailscale: command not found` (Mac) | See the alias in Step 3. |
| Mac: service won't start | Check `~/Library/Logs/localdrive.log`. Make sure the binary is in `~/bin`, not inside OneDrive, and that `-root` isn't Documents, Desktop or Downloads. |
| Windows: task shows "Running" but nothing responds | Run the command from Step 2 by hand to see the error message. It's usually a wrong `-root` path or port 8080 already in use. |
| Port 8080 already in use | Pick another port, e.g. `-addr 127.0.0.1:8090`, then point Serve at it: run `tailscale serve reset`, then `tailscale serve --bg 8090`. |
| Uploads from the phone stop partway | Keep the browser in the foreground with the screen on until the upload finishes. |
| Slow transfers when away from home | Run `tailscale status` and look for `relay` next to the phone. That means traffic is going through Tailscale's relay servers instead of directly between your devices. It still works, just slower; try another Wi-Fi or mobile network. |
