; The app's own autostart entry is per user, so the per-user uninstaller
; can remove it. MyGo calls customUnInstall before deleting $INSTDIR.
!macro customUnInstall
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "io.github.qijyc101.qbox"
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run" "io.github.qijyc101.qbox"
!macroend
