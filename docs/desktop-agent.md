# ClusterStor desktop agent

## Product role

The desktop agent brings the ClusterStor account into the native computer experience. It is one agent regardless of which storage provider ultimately holds a file.

The Windows implementation is first. macOS follows after the Windows lifecycle and sync engine are stable.

## Current foundation

The current Windows agent slice provides:

- per-user agent configuration under the user's local ClusterStor state directory;
- explicit Peer Storage onboarding, with contribution off by default;
- browser-based device pairing instead of storing a normal ClusterStor account session;
- a short human-readable pairing code plus a separate high-entropy polling token;
- signed-in approval from the ClusterStor Devices screen;
- one-time device credential issuance after approval;
- Windows DPAPI protection for the device secret at rest;
- device-authenticated heartbeats that update `last_seen_at`;
- revocation through the existing Devices screen;
- Windows cross-compilation in CI;
- an Inno Setup developer-preview installer definition.

The agent does not yet synchronize files. This is intentional: device identity, credentials, revocation, and lifecycle are established before filesystem mutation is introduced.

## Pairing flow

1. The agent starts on an unregistered computer.
2. Setup explicitly asks whether the user wants to participate in Peer Storage.
3. If Peer Storage is declined, contribution remains off with zero local capacity.
4. If Peer Storage is accepted, setup immediately asks how much local disk space to contribute.
5. The agent requests a pairing from the API and receives:
   - a short user code;
   - a high-entropy poll token;
   - a browser verification URL;
   - an expiration time.
6. The agent opens the verification URL.
7. If necessary, the user signs in and is returned to the pairing request.
8. The Devices page shows the requesting computer, platform, agent version, and Peer choice.
9. The user explicitly approves the device.
10. The agent polls with the high-entropy token, receives its device credential, and stores that secret with Windows DPAPI.
11. The agent sends periodic device-authenticated heartbeats.

The account password, normal browser session token, provider OAuth tokens, and device secret must never be written to logs.

## Windows build

From PowerShell at the repository root:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows-agent.ps1
```

The executable is written to:

```text
dist\windows\clusterstor-agent.exe
```

The current executable is a console-based developer preview. A tray UI will replace the console onboarding before production release.

## Installer

`packaging/windows/ClusterStor.iss` is an Inno Setup definition for the developer-preview installer.

After building the agent, compile that script with Inno Setup to produce:

```text
dist\windows\ClusterStorSetup.exe
```

The installer is per-user and does not require administrator privileges. Automatic startup is currently an optional installer task until the tray/background lifecycle is complete.

## Tray and Settings UX

The production Windows tray menu should stay focused on day-to-day operation:

- Open ClusterStor folder
- Sync status
- Pause / Resume sync
- Activity
- Providers
- Devices
- Settings
- Manage plan...
- Exit ClusterStor

`Manage plan...` is a convenience shortcut that opens the signed-in web billing screen. Full billing administration belongs under **Settings > Billing** in the ClusterStor account experience rather than being implemented as a complex native tray workflow.

Settings should eventually include:

- General
- Sync
- Providers
- Devices
- Peer Storage
- Notifications
- Billing
- About / Updates

Billing should cover the current plan, provider-adapter entitlement, ClusterStor Cloud capacity, Peer Storage plan, renewal cycle, payment method, invoices/receipts, upgrade/downgrade, and cancellation. The desktop agent must not store payment-card data or billing-provider credentials.

## Next desktop milestones

1. tray/background lifecycle and Windows startup behavior;
2. local ClusterStor folder creation;
3. local metadata database and sync journal;
4. filesystem watcher;
5. provider-neutral sync operation interface;
6. Google Drive upload/download/change application;
7. conflict detection and version creation;
8. resumable transfer queue;
9. selective/offline sync behavior;
10. production installer signing and automatic updates.

Peer Storage networking is not part of the initial sync-engine milestone. Only explicit onboarding and local configuration are established now.
