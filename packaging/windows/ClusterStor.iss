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
Filename: "{app}\{#MyAppExeName}"; Parameters: "--drive-name ""{code:GetDriveName}"" --drive-letter ""{code:GetDriveLetter}"""; Description: "Pair this computer with ClusterStor"; Flags: postinstall nowait skipifsilent

[Code]
var
  DrivePage: TInputQueryWizardPage;

procedure InitializeWizard;
begin
  DrivePage := CreateInputQueryPage(
    wpSelectTasks,
    'ClusterStor Drive',
    'Choose how ClusterStor appears in File Explorer',
    'ClusterStor will appear under This PC as a mapped drive. You can change these settings later.'
  );
  DrivePage.Add('Drive name:', False);
  DrivePage.Values[0] := 'ClusterStor';
  DrivePage.Add('Drive letter:', False);
  DrivePage.Values[1] := 'S';
end;

function NormalizeDriveLetter(Value: String): String;
begin
  Result := Uppercase(Trim(Value));
  if (Length(Result) = 2) and (Result[2] = ':') then
    Delete(Result, 2, 1);
end;

function NextButtonClick(CurPageID: Integer): Boolean;
var
  Letter: String;
begin
  Result := True;
  if CurPageID <> DrivePage.ID then
    Exit;

  if Trim(DrivePage.Values[0]) = '' then
  begin
    MsgBox('Enter a name for the ClusterStor drive.', mbError, MB_OK);
    Result := False;
    Exit;
  end;

  Letter := NormalizeDriveLetter(DrivePage.Values[1]);
  if (Length(Letter) <> 1) or (Letter[1] < 'A') or (Letter[1] > 'Z') then
  begin
    MsgBox('Enter a single drive letter from A to Z.', mbError, MB_OK);
    Result := False;
    Exit;
  end;

  if DirExists(Letter + ':\') then
  begin
    MsgBox('Drive ' + Letter + ': is already in use. Choose another letter.', mbError, MB_OK);
    Result := False;
    Exit;
  end;

  DrivePage.Values[1] := Letter;
end;

function GetDriveName(Param: String): String;
begin
  Result := Trim(DrivePage.Values[0]);
end;

function GetDriveLetter(Param: String): String;
begin
  Result := NormalizeDriveLetter(DrivePage.Values[1]);
end;
