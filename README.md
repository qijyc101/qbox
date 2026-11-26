# qbox

Нативный клиент VPN/прокси для Windows на Go: интерфейс на
[MyGo](https://github.com/egoist/mygo) (`ui`, без web-view), ядро
[sing-box](https://github.com/SagerNet/sing-box) подключено как Go-библиотека,
а не как отдельный процесс.

## Статус

MVP для Windows 10/11 x64:

- TUN-режим с автоматической маршрутизацией и перехватом DNS;
- подписки в формате base64/URI-списка и sing-box JSON;
- VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC, SOCKS, HTTP;
- режимы Global / Rules / Direct;
- обход локальной сети, обход РФ по rule-sets, блокировка рекламы;
- свои списки доменов и CIDR для proxy/direct;
- статистика трафика и журнал sing-box в окне;
- конфигурация в `%AppData%\qbox\config.json`.

Clash YAML и мобильные платформы в этот MVP не входят.

## Требования

- Windows 10/11 x64 (arm64 собирается, но не проверялся);
- Go 1.27.4+;
- права администратора для TUN: создание Wintun-адаптера и изменение
  таблицы маршрутов требуют elevation.

## Сборка

```powershell
.\scripts\build.ps1
```

Скрипт выставляет обязательные build tags sing-box и вызывает MyGo:

```powershell
$env:GOFLAGS = "-tags=with_gvisor,with_quic,with_utls,with_grpc"
go tool mygo build -platform windows/amd64 -skip-installer
```

Без `with_gvisor` не будет mixed-стека TUN, без `with_quic` — Hysteria2/TUIC,
без `with_utls` — uTLS/REALITY.

Результат: `build/windows-amd64/qbox.exe` и `wintun.dll` рядом с ним.

## Wintun

Официальные подписанные DLL Wintun 0.14.1 уже лежат в
`resources/windows-amd64` и `resources/windows-arm64`. Обновить их:

```powershell
.\scripts\fetch-wintun.ps1
```

`wintun.dll` загружается до старта sing-box по полному пути из ресурсов,
поэтому sing-tun находит его без копирования в системные каталоги.

## Запуск

1. Запустите `qbox.exe`.
2. Откройте «Серверы» → «Добавить подписку», вставьте URL.
3. Выберите ноду или «Авто».
4. При первом подключении Windows запросит UAC: приложение перезапустится
   с правами администратора и автоматически подключится.

Приложение работает как один экземпляр; повторный запуск передаёт управление
первому.

## Маршрутизация

- **Global** — весь трафик через выбранную ноду.
- **Rules** — правила и списки, финальный маршрут через ноду.
- **Direct** — без прокси, системный DNS.

Порядок правил: sniff → трафик самого qbox напрямую → блокировка рекламы →
LAN напрямую → свои proxy-списки → свои direct-списки → обход РФ → финальный
маршрут. Смена ноды перезапускает ядро; изменения маршрутизации применяются
кнопкой «Перезапустить».

Rule-sets скачиваются с `SagerNet/sing-geosite` и `SagerNet/sing-geoip` в
`%LocalAppData%\qbox\rulesets` и кешируются с ETag.

## Файлы

| Что | Где |
|---|---|
| Конфигурация | `%AppData%\qbox\config.json` |
| Резервная копия | `%AppData%\qbox\config.json.bak` |
| Кэш sing-box | `%LocalAppData%\qbox\cache\cache.db` |
| Rule-sets | `%LocalAppData%\qbox\rulesets` |
| Журнал | `%AppData%\qbox\logs\qbox.log` |

## Тесты

```powershell
.\scripts\test.ps1
```

Скрипт прогоняет unit-тесты, `go vet`, `mygo vet`, а тест UI собирает в
`build/test` и запускает оттуда (в некоторых конфигурациях Windows Defender
блокирует тестовый exe в `%TEMP%`).

Проверка TUN выполняется вручную на машине с правами администратора и
реальной подпиской: IP/DNS-leak, LAN, обход РФ, adblock, смена ноды.

## Ограничения

- sing-box линкуется как библиотека, поэтому qbox распространяется под GPLv3.
- Изменение ноды/маршрутизации в MVP перезапускает ядро и рвёт соединения.
- Нет системного прокси, ping-теста нод и hot-switch без перезапуска.
- Нет автоматического запуска при входе: elevated-автозапуск требует
  Task Scheduler и отдельного этапа.
- Неподписанный exe может вызывать предупреждение SmartScreen.

## Лицензия

qbox — GNU GPL v3.0 или позднее, см. `LICENSE`.

sing-box — GPLv3 с дополнительным условием о запрете использования имени;
MyGo — MIT; Wintun — лицензия на официальные подписанные бинарники.
Подробности в `NOTICE`.
