# Bot manager capability

`bot_can_manage_bots` remains disabled by default. This one-off operations command is the only supported grant path; there is no client-facing API for changing it.

Apply `migrate-20261008-bot-manager-hierarchy.sql` before running the command. The operator's MySQL account needs `SELECT` on `users` and `bots`, `UPDATE` on `bots`, and `INSERT` on `bot_manager_capability_audit`.

Keep the DSN in the operations runner's secret store and invoke an explicit operation with a change reason:

```sh
go run ./app/service/biz/user/cmd/bot-manager grant --bot-id 12345 --reason "OPS-1234 approved bot delegation"
go run ./app/service/biz/user/cmd/bot-manager revoke --bot-id 12345 --reason "OPS-1234 delegation ended"
```

The command uses MySQL `CURRENT_USER()` as the operator identity and writes each state transition and reason to the audit table in the same transaction. Repeating the current state is idempotent and does not add another audit row.
