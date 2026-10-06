; ClusterStor Windows installer (developer preview)
#define MyAppName "ClusterStor"
#define MyAppVersion "0.1.0-dev"
#define MyAppPublisher "ClusterStor"
#define MyAppExeName "clusterstor-agent.exe"

[Setup]
AppId={{B4D2B5D3-3490-4D41-9A39-BED3C4CBBC6A}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={localappdata}\Programs\ClusterStor
DefaultGroupName=ClusterStor
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\..\dist\windows
OutputBaseFilename=ClusterStorSetup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\{#MyAppExeName}

[Files]
Source: "..\..\dist\windows\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion

[Tasks]
Name: "startup"; Description: "Start ClusterStor when I sign in to Windows"; GroupDescription: "Startup:"; Flags: unchecked

[Icons]
Name: "{group}\ClusterStor"; Filename: "{app}\{#MyAppExeName}"
Name: "{userstartup}\ClusterStor"; Filename: "{app}\{#MyAppExeName}"; Tasks: startup

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "Pair this computer with ClusterStor"; Flags: postinstall nowait skipifsilent
