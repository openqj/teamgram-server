# APIFull language packs

The JSON files in `weba/` are build-time exports from Telegram's official
translation catalog. They are embedded into the APIFull binary and served by
the standard `langpack.*` methods; production startup does not contact Telegram.

To refresh or add a WebA language, run this from `teamgram-server`:

```sh
node tools/langpack_import.mjs en zh-hans zh-hant ru es de fr ja ko
```

Add the language metadata to `languages.json` when adding a new file. Increase
the pack `version` when changing an already deployed language pack so clients
request a new difference.
