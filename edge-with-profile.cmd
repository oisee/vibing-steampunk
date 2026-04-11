@echo off
REM Launch Edge with default user profile so it has existing IAS/SAML session
"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe" --user-data-dir="C:\Users\stanislav.naumov\AppData\Local\Microsoft\Edge\User Data" %*
