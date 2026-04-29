@echo off
REM 学童 管理サーバー 起動スクリプト
REM このファイルをスタッフPCのスタートアップフォルダに入れると PC 起動時に自動で立ち上がる
REM
REM スタートアップフォルダの場所:
REM   Win + R → shell:startup → Enter → このファイルのショートカットを置く

cd /d "%~dp0"
start "" /min server.exe
