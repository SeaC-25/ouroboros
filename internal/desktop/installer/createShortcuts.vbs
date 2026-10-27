' createShortcuts.vbs - Windows shortcut creation without PowerShell
' execution-policy friction. Invoked by the Go installer via cscript.
Dim shell, startMenuPrograms, desktop, link
Set shell = CreateObject("WScript.Shell")

startMenuPrograms = shell.SpecialFolders("Programs")
Set link = shell.CreateShortcut(startMenuPrograms & "\Ouroboros.lnk")
link.TargetPath = WScript.Arguments(0)
link.WorkingDirectory = WScript.Arguments(1)
link.Description = "Ouroboros security workbench"
link.Save

desktop = shell.SpecialFolders("Desktop")
Set link = shell.CreateShortcut(desktop & "\Ouroboros.lnk")
link.TargetPath = WScript.Arguments(0)
link.WorkingDirectory = WScript.Arguments(1)
link.Description = "Ouroboros security workbench"
link.Save

WScript.Echo "shortcuts created"
