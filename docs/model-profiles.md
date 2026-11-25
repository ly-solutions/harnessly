# Профили моделей без пересборки

Встроенные пресеты (Kimi, GLM, Grok, GPT, Авто) остаются как есть. Профили из файла или от адаптера **дополняют** их;
профиль с тем же `id`, что у встроенного, **заменяет** его на том же месте. Пока файл и адаптер не заданы, ничего не
меняется.

## Включение

| Настройка среды рантайма | Что делает |
|---|---|
| `RUNTIME_PROFILES_FILE` | путь к файлу профилей (JSON) |
| `RUNTIME_PROFILES_ADAPTER_URL`, `RUNTIME_PROFILES_ADAPTER_TOKEN_FILE` | адаптер протокола расширений, отдаёт тот же документ |
| `RUNTIME_PROFILES_SECRET_CMD` | команда чтения секрета сейфа: `{name}` заменяется именем, значение берётся из stdout (например `my-safe get {name}`) |

Пример: `examples/profiles/profiles.json` (GLM со своим ключом и корпоративный прокси `corp-llm-proxy.example.com`).

## Формат профиля

- `id` (`a-z`, цифры, дефис), `label`, `vendor` (`kimi`, `glm`, `grok`, `gpt`: от него зависит значок на экране).
- `api`: `anthropic` (идёт через Claude Code) или `openai-responses` (через Codex, тогда нужен `key_env`).
- `base_url`: только https (http допустим для localhost), без логина, query и фрагмента.
- `key_ref`: **имя** секрета сейфа, не значение. Имя, похожее на ключ (`sk-…`), файл не пропускает. Ключ читается через
  `RUNTIME_PROFILES_SECRET_CMD`, не чаще раза в минуту, и кладётся в файл ключа профиля (0600), как введённый вручную.
  Недоступный сейф оставляет прежний ключ. Без `key_ref` ключ вводится на экране «Настройки».
- `models[]`: `id`, `label`, `hint`, `window` (окно контекста), `efforts`, `default_effort`, `price` (доллары за миллион
  токенов: `input`, `output`, `cached_input`). Цена попадает в расчёт стоимости. `id` модели — до 128 символов:
  латиница, цифры, `.`, `_`, `:`, `-`, до четырёх частей через `/` (`zai/glm-5.3`, `openrouter/moonshotai/Kimi-K2`).
  Другой `id` файл не примет и скажет причину, а не покажет модель в выборе, чтобы потом отказать в запуске.
- `small`, `test`: модели для фоновой работы и проверки соединения (по умолчанию первая).
- `env`: добавочные переменные среды CLI. `ANTHROPIC_API_KEY` и `ANTHROPIC_AUTH_TOKEN` запрещены.

Один неверный профиль отклоняет весь файл целиком: половины профилей не бывает. Ошибка идёт в stderr рантайма, а
встроенные пресеты работают.

## Адаптер

Протокол `harnessly.model-profiles/v1` (см. `docs/hub/extensions.md`): `GET /v1/describe` с возможностью `profiles` и
`GET /v1/profiles` с документом того же формата. Профили загружаются при старте рантайма.

## Чего пока нет

- Значка «своего» производителя: `vendor` берётся из четырёх существующих (новое значение меняет схему протокола).
- Перечитывания файла без перезапуска рантайма.
- Цен для моделей Claude-формата: цена учитывается в расчёте по имени модели через таблицу OpenAI-стиля.

### Start a Claude profile without MCP

Set `"disable_mcp": true` on an `anthropic` file profile to start and resume its chats with Claude Code's strict MCP configuration and no explicit servers. The Hub catalog remains available to other profiles. Shell execution continues to use the existing permissions, hooks and sandbox. This option does not authorize additional commands or provider operations.

### Leave built-in MCP tools out of a launch

Some providers reject a whole request when its tool list names tools they do not support, such as the built-in media tools. Set `"exclude_builtin_mcp"` on an `anthropic` file profile to choose what is advertised at launch, for new and resumed chats alike:

- `"media"`: the image, video and audio tools of the built-in server are denied, so the model is not offered them. The other built-in tools stay.
- `"runtime"`: the built-in runtime server is not given to the chat.
- `"apps"`: the apps' MCP servers are not given to the chat.

Servers the person or the provider configured stay, so an explicitly allowed external integration keeps working; unlike `disable_mcp`, no strict MCP mode is used. A profile without the field behaves as before, and the field only ever removes tools: it cannot add servers or widen permissions. `disable_mcp` takes precedence. The agent's launch journal entry lists the excluded groups and the number of tools hidden, never addresses, tokens or request contents.
