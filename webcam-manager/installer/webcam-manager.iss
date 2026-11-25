; Installer Windows di Webcam Manager (Inno Setup 6).
; Compilazione:  iscc /DAppVersion=1.2.0 installer\webcam-manager.iss
; Richiede in installer\files\: webcam-manager.exe e, facoltativo, ffmpeg.exe.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

[Setup]
AppId={{6C1E5E0B-3D7B-4B8E-9C61-2A8F4D0B7E21}
AppName=Webcam Manager
AppVersion={#AppVersion}
AppPublisher=Paoli Luca
AppCopyright=made by Paoli Luca 2026 - paoli.lu@gmail.com
AppPublisherURL=mailto:paoli.lu@gmail.com
AppSupportURL=mailto:paoli.lu@gmail.com
VersionInfoCompany=Paoli Luca
VersionInfoCopyright=made by Paoli Luca 2026
DefaultDirName={autopf}\Webcam Manager
DefaultGroupName=Webcam Manager
DisableProgramGroupPage=yes
OutputDir=..\dist
OutputBaseFilename=WebcamManager-Setup-{#AppVersion}
Compression=lzma2
SolidCompression=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern
UninstallDisplayIcon={app}\webcam-manager.exe
CloseApplications=no

[Languages]
Name: "it"; MessagesFile: "compiler:Languages\Italian.isl"

[Files]
Source: "files\webcam-manager.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "files\ffmpeg.exe"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist

[Registry]
Root: HKLM; Subkey: "Software\WebcamManager"; Flags: uninsdeletekey

[Icons]
Name: "{group}\Pannello Webcam Manager"; Filename: "http://localhost:{code:GetPort}/"
Name: "{group}\Disinstalla Webcam Manager"; Filename: "{uninstallexe}"

[Run]
; nuova installazione: registra il servizio con i dati inseriti
Filename: "{app}\webcam-manager.exe"; \
  Parameters: "install -data ""{commonappdata}\WebcamManager"" -listen :{code:GetPort} -admin-password ""{code:GetPassword}"" -location ""{code:GetLocation}"" -altitude {code:GetAltitude}"; \
  StatusMsg: "Configurazione del servizio di Windows..."; Flags: runhidden waituntilterminated; Check: not IsUpgrade
; aggiornamento: il servizio esiste già, basta riavviarlo
Filename: "{sys}\sc.exe"; Parameters: "start WebcamManager"; Flags: runhidden waituntilterminated; Check: IsUpgrade
Filename: "http://localhost:{code:GetPort}/"; Description: "Apri il pannello di Webcam Manager"; Flags: postinstall shellexec nowait skipifsilent

[UninstallRun]
Filename: "{app}\webcam-manager.exe"; Parameters: "uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "RemoveService"

[Code]
var
  LocPage: TInputQueryWizardPage;
  AccessPage: TInputQueryWizardPage;
  Upgrade: Boolean;

function IsUpgrade: Boolean;
begin
  Result := Upgrade;
end;

procedure InitializeWizard;
begin
  Upgrade := RegKeyExists(HKLM, 'SYSTEM\CurrentControlSet\Services\WebcamManager');
  if Upgrade then exit; // in aggiornamento si mantiene la configurazione esistente

  LocPage := CreateInputQueryPage(wpSelectDir,
    'Località', 'Dove si trovano le webcam?',
    'Questi dati compaiono sulle immagini, nel titolo dello storico e nelle email di avviso. Si possono cambiare in seguito dal pannello.');
  LocPage.Add('Nome della località (es. Camping Coggiolo Sant''Anna Pelago (MO)):', False);
  LocPage.Add('Altitudine in metri sul livello del mare:', False);

  AccessPage := CreateInputQueryPage(LocPage.ID,
    'Accesso al pannello', 'Protezione del pannello web',
    'Il pannello si apre dal browser. Utente: admin.');
  AccessPage.Add('Password del pannello:', True);
  AccessPage.Add('Porta del pannello:', False);
  AccessPage.Values[1] := '8080';
end;

function NextButtonClick(CurPageID: Integer): Boolean;
var
  n: Integer;
begin
  Result := True;
  if Upgrade then exit;
  if CurPageID = LocPage.ID then begin
    if Trim(LocPage.Values[0]) = '' then begin
      MsgBox('Inserisci il nome della località.', mbError, MB_OK); Result := False; exit;
    end;
    n := StrToIntDef(Trim(LocPage.Values[1]), -1);
    if (n < 0) or (n > 9000) then begin
      MsgBox('Inserisci l''altitudine in metri (es. 1250).', mbError, MB_OK); Result := False; exit;
    end;
  end;
  if CurPageID = AccessPage.ID then begin
    if Length(AccessPage.Values[0]) < 6 then begin
      MsgBox('La password deve avere almeno 6 caratteri.', mbError, MB_OK); Result := False; exit;
    end;
    if Pos('"', AccessPage.Values[0]) > 0 then begin
      MsgBox('La password non può contenere virgolette.', mbError, MB_OK); Result := False; exit;
    end;
    n := StrToIntDef(Trim(AccessPage.Values[1]), 0);
    if (n < 1) or (n > 65535) then begin
      MsgBox('Porta non valida.', mbError, MB_OK); Result := False; exit;
    end;
  end;
end;

function Clean(const S: String): String;
begin
  Result := S;
  StringChangeEx(Result, '"', '', True);
end;

function GetLocation(Param: String): String;
begin
  if Upgrade then Result := '' else Result := Clean(Trim(LocPage.Values[0]));
end;

function GetAltitude(Param: String): String;
begin
  if Upgrade then Result := '-1' else Result := IntToStr(StrToIntDef(Trim(LocPage.Values[1]), 0));
end;

function GetPassword(Param: String): String;
begin
  if Upgrade then Result := '' else Result := AccessPage.Values[0];
end;

function GetPort(Param: String): String;
var
  s: String;
begin
  if Upgrade then begin
    // la porta scelta alla prima installazione è salvata nel registro
    if not RegQueryStringValue(HKLM, 'Software\WebcamManager', 'Port', s) then s := '8080';
    Result := s;
  end else
    Result := Trim(AccessPage.Values[1]);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if (CurStep = ssPostInstall) and not Upgrade then
    RegWriteStringValue(HKLM, 'Software\WebcamManager', 'Port', GetPort(''));
end;

// prima di sostituire l'eseguibile, in aggiornamento, ferma il servizio
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  rc, i: Integer;
begin
  Result := '';
  if Upgrade then begin
    Exec(ExpandConstant('{sys}\sc.exe'), 'stop WebcamManager', '', SW_HIDE, ewWaitUntilTerminated, rc);
    for i := 1 to 20 do Sleep(500);
  end;
end;
