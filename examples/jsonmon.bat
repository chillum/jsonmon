@echo off
rem jsonmon start-up script for Windows
rem Can be used from Task Manager, NSSM and others

rem HTTP host & port
rem Set host to [::] to listen on all interfaces
set HOST=localhost
set PORT=3000

cd %SYSTEMDRIVE%\jsonmon
jsonmon.exe -syslog config.yml
