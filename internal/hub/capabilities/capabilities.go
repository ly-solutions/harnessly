// Package capabilities is the list behind the screen «Возможности» of an organization's settings: every part of the hub
// that is switched on or off, in plain words, with its state and where it is set. It reads what the hub already knows:
// the instance's settings (HUB_*, read-only here), the organization's flags (internal/hub/features) and the rollout
// switches (internal/hub/switches). It changes nothing; a flag and a rollout switch are changed through their own
// endpoints, by an administrator of the installation.
package capabilities

import (
	"context"
	"slices"

	"github.com/ly-solutions/harnessly/internal/hub/config"
	"github.com/ly-solutions/harnessly/internal/hub/switches"
)

// Source says where a capability is decided.
type Source string

const (
	// Env: the instance's settings file (a HUB_* variable); the hub cannot change it, the screen only shows it.
	Env Source = "env"
	// Org: a flag of the organization (PUT /api/orgs/{org}/features/{name}).
	Org Source = "org"
	// Rollout: a switch of the whole installation (PUT /api/admin/switches).
	Rollout Source = "rollout"
)

// Item is one row of the screen.
type Item struct {
	// ID is the variable, the flag or the switch: what it is called in the settings and the API.
	ID    string `json:"id"`
	Group string `json:"group"`
	// Name and Description are for the person: Russian, no internal words.
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      Source `json:"source"`
	Enabled     bool   `json:"enabled"`
	// Detail is a short state when on/off is not all of it («только предупреждает»); empty otherwise.
	Detail string `json:"detail,omitempty"`
	// DefaultOn says the hub turns it on itself when nobody chose.
	DefaultOn bool `json:"defaultOn"`
	// Needs says, in words, what else it takes to work: a service, a domain, a secret. Empty: nothing.
	Needs string `json:"needs,omitempty"`
	// Why says why it is off by default; empty for what is on by default.
	Why string `json:"why,omitempty"`
	// Forced says the instance decides a flag whatever the organization chose.
	Forced bool `json:"forced,omitempty"`
	// SetBy is where to change a switch the screen cannot: the variable of the hub's settings.
	SetBy string `json:"setBy,omitempty"`
	// CanToggle says the viewer may switch it from the screen: a flag or a rollout switch, not forced, and the viewer is
	// an administrator of the installation.
	CanToggle bool `json:"canToggle"`
}

// Gates are what the list reads besides the settings.
type Gates struct {
	// Org answers an organization's flag and whether the instance forces it.
	Org interface {
		On(ctx context.Context, orgID, name string) bool
		Forced(name string) bool
	}
	// Rollout answers a rollout switch for everyone (no person, no machine).
	Rollout switches.Gate
}

// def is one entry of the catalog.
type def struct {
	id, group, name, desc string
	source                Source
	// on is the state of an env entry from the settings; detail is its short state.
	on     func(c *config.Config) bool
	detail func(c *config.Config) string
	// defaultOn: it is on when nobody chose.
	defaultOn bool
	needs     string
	why       string
	// viaEnv: a flag the settings can switch on for all organizations (HUB_ENV_FEATURES): variable name.
	viaEnv string
}

// Groups in the order the screen shows them.
const (
	GroupAuto    = "Регулярные задачи и автоматизация"
	GroupMachine = "Машины и агенты"
	GroupSpend   = "Расходы и лимиты"
	GroupEnv     = "Окружение агентов"
	GroupDocs    = "Документы, макеты и ссылки"
	GroupHub     = "Данные и поиск на хабе"
	GroupUI      = "Внешний вид"
	GroupSafe    = "Секреты"
)

// Groups is the order of the groups.
var Groups = []string{GroupAuto, GroupMachine, GroupSpend, GroupEnv, GroupDocs, GroupHub, GroupUI, GroupSafe}

var catalog = []def{
	// Регулярные задачи и автоматизация
	{id: "HUB_AUTOMATIONS", group: GroupAuto, source: Env, defaultOn: true,
		name: "Регулярные задачи и автоматизации",
		desc: "Вы просите агента в чате: «каждое утро присылай дайджест» или «следи за выкатом», — и хаб сам запускает такую задачу на вашей машине в новом чате, проверяет отчёт и сообщает о результате. Хаб только запускает и сообщает, работу делает агент. Правила «когда случилось событие — запусти чат» работают там же.",
		on:   func(c *config.Config) bool { return c.Automations }},
	{id: "automation-triggers", group: GroupAuto, source: Rollout, defaultOn: true,
		name:  "Запуск по расписанию и по ожиданию",
		desc:  "Регулярные задачи и правила срабатывают по времени («по будням в 9:00») и когда нужное событие не пришло за заданный срок. Без этого работают только правила, которые ждут событие снаружи.",
		needs: "Включённые «Регулярные задачи и автоматизации»."},
	{id: "prompt-launch", group: GroupAuto, source: Rollout,
		name:  "Запуск чата снаружи",
		desc:  "Внешний сервис, например бот в мессенджере, запускает чат на вашей машине по персональному токену.",
		needs: "Токен запуска, который администратор установки задаёт в настройках хаба (HUB_LAUNCH_TOKEN).",
		why:   "Без токена запуска дверь закрыта, а токен — секрет, который нужно выдать отдельно."},
	{id: "HUB_EVENT_BUS", group: GroupAuto, source: Env,
		name: "Журнал событий",
		desc: "Всё, что случилось у человека — письмо, сообщение, назначенная задача, упавшая сборка, — попадает в один журнал, и из него берут автоматизации, дайджест и уведомления. События хранятся 30 дней.",
		why:  "Складывает в базу хаба содержимое событий людей: оператор решает, нужно ли это хранить.",
		on:   func(c *config.Config) bool { return c.EventBus }},

	// Машины и агенты
	{id: "HUB_PEERS", group: GroupMachine, source: Env,
		name: "Переписка агентов между машинами",
		desc: "Агент на одной машине видит чаты других машин вашего пространства и пишет в них. Принимать сообщение или нет, решает настройка самой машины.",
		why:  "Агенты разных людей получают общий канал, а ограничение доступа к машинам по умолчанию не включено.",
		on:   func(c *config.Config) bool { return c.Peers }},
	{id: "HUB_MEMORY_SYNC", group: GroupMachine, source: Env,
		name:  "Память агента на всех машинах",
		desc:  "Заметки, которые агент запоминает о проекте, доезжают до остальных ваших машин через хаб. Чужие машины их не видят.",
		needs: "Настройка на самой машине: хаб один ничего не синхронизирует.",
		why:   "Заметки памяти могут содержать личное, и хранить их в базе хаба должен решить оператор.",
		on:    func(c *config.Config) bool { return c.Memory }},
	{id: "HUB_VOICE_TAKES", group: GroupMachine, source: Env,
		name: "Записи диктовки на хабе",
		desc: "Надиктованные в чате записи остаются на хабе и видны на всех ваших устройствах, а не только в браузере, где их записали.",
		why:  "Хаб хранит звук людей: до 500 записей на человека, каждая до 8 МБ.",
		on:   func(c *config.Config) bool { return c.VoiceTakes }},
	{id: "machine.seats", group: GroupMachine, source: Org, defaultOn: true,
		name: "Один сервер — несколько мест",
		desc: "Машины на одном сервере показываются одним сервером с местами людей, а нагрузка и диск считаются один раз. Меняется только экран «Машины»."},
	{id: "machine.resources", group: GroupMachine, source: Org,
		name:  "Ресурсы и цена машин",
		desc:  "Можно менять ресурсы машины и видеть их цену.",
		needs: "Служба облака, которая знает ваши серверы (HUB_PROVIDER_URL).",
		why:   "Без службы облака править нечего."},
	{id: "runtime.fleet", group: GroupMachine, source: Org,
		name: "Один рантайм на все машины человека",
		desc: "Человек открывает все свои машины в одной рабочей области, а хаб сам выбирает, на какой запустить чат.",
		why:  "Меняет путь, по которому хаб открывает машины, поэтому включается осознанно."},
	{id: "chat.resume", group: GroupMachine, source: Org,
		name:  "Продолжить чат на другой машине",
		desc:  "Если машина, на которой шёл чат, выключена, чат продолжается на другой вашей машине с последнего снимка в хабе.",
		needs: "Включённый «Один рантайм на все машины человека».",
		why:   "Хаб хранит снимки чатов людей."},
	{id: "HUB_REPLICAS", group: GroupMachine, source: Env,
		name:  "Несколько копий хаба",
		desc:  "Хаб работает в нескольких экземплярах за одним адресом, и они передают друг другу связь с машинами.",
		needs: "База данных Postgres и несколько экземпляров хаба.",
		why:   "Для одного сервера не нужно.",
		on:    func(c *config.Config) bool { return c.Replicas }},
	{id: "HUB_TBOT_JOIN", group: GroupMachine, source: Env,
		name:  "Выдача доступа машине через Teleport",
		desc:  "Машина просит у хаба токен для своей учётной записи в Teleport, а человек подтверждает это на экране «Машины».",
		needs: "Кластер Teleport и Kubernetes, к которым хаб имеет доступ.",
		why:   "Нужна чужая инфраструктура.",
		on:    func(c *config.Config) bool { return c.TbotJoin.On }},
	{id: "tbot-join", group: GroupMachine, source: Rollout,
		name:  "Выдача доступа машине через Teleport: разрешение людям",
		desc:  "Разрешает людям просить такой токен. Работает вместе с настройкой выше.",
		needs: "Выдача доступа через Teleport в настройках хаба.",
		why:   "Нужна чужая инфраструктура."},
	{id: "client-api", group: GroupMachine, source: Rollout,
		name: "Личные приложения по токену",
		desc: "Человек подключает свои приложения (лаунчер, счётчик в строке меню) личным токеном, и они читают и пишут его чаты.",
		why:  "Выдаёт приложениям доступ к чатам человека: включается осознанно."},
	{id: "preview-slot-host", group: GroupMachine, source: Rollout,
		name:  "Предпросмотр на отдельном адресе",
		desc:  "Страницы, которые агент запускает на машине для просмотра, открываются на отдельном адресе, а не внутри хаба.",
		needs: "Отдельный домен с общей записью в DNS и сертификатом (HUB_PREVIEW_SLOT_DOMAIN).",
		why:   "Нужен домен."},

	// Расходы и лимиты
	{id: "HUB_USAGE_INGEST", group: GroupSpend, source: Env,
		name: "Учёт расходов с машин",
		desc: "Хаб забирает с машин факты расходов: сколько было вызовов моделей, токенов и денег, кто и на какой подписке работал. Текст разговоров не передаётся. Из этого строятся «Аналитика» и лимиты.",
		why:  "Собирает по каждому человеку, кто и сколько работал: оператор решает, нужно ли это людям.",
		on:   func(c *config.Config) bool { return c.UsageIngest }},
	{id: "HUB_LIMITS_ENFORCE", group: GroupSpend, source: Env,
		name: "Лимиты расходов на машинах",
		desc: "Что делают машины с лимитами: ничего не знают о них, только предупреждают или не пускают в новые чаты.",
		why:  "Может остановить людям работу, поэтому включается осознанно.",
		on:   func(c *config.Config) bool { return c.LimitsEnforce != "off" },
		detail: func(c *config.Config) string {
			switch c.LimitsEnforce {
			case "warn":
				return "Только предупреждает"
			case "stop":
				return "Не пускает в новые чаты"
			}
			return ""
		}},
	{id: "efficiency", group: GroupSpend, source: Org,
		name:  "Эффективность",
		desc:  "Цена слитого запроса на изменения, что чинить первым, как меняется работа агентов.",
		needs: "Хост кода (GitLab), откуда хаб читает слитые запросы.",
		why:   "Без хоста кода показывать нечего."},
	{id: "hub-stats", group: GroupSpend, source: Rollout,
		name:  "Статистика из хаба",
		desc:  "Экран статистики человека берёт цифры из хаба, а не с одной машины, и показывает все машины сразу.",
		needs: "Включённый «Учёт расходов с машин».",
		why:   "Без учёта расходов цифр в хабе нет, а статистика самой машины при этом убирается."},

	// Окружение агентов
	{id: "env", group: GroupEnv, source: Org, viaEnv: "HUB_ENV_FEATURES",
		name: "Окружение машин",
		desc: "Экран «Окружение»: какие репозитории, доступы, подключения и скиллы получают машины организации, по слоям организация → команда → человек.",
		why:  "Меняет то, что ставится на машины людей, поэтому включается осознанно."},
	{id: "catalog", group: GroupEnv, source: Org, viaEnv: "HUB_ENV_FEATURES",
		name: "Каталог инструментов и скиллов",
		desc: "Общий каталог скиллов, подключений и репозиториев, из которого администраторы берут нужное в окружение.",
		why:  "Работает вместе с «Окружением машин»."},
	{id: "env.own", group: GroupEnv, source: Org,
		name:  "Личный слой окружения",
		desc:  "Человек добавляет свои скиллы и подключения только на свои машины и выключает для себя то, что не нужно.",
		needs: "Включённое «Окружение машин».",
		why:   "Меняет то, что ставится на машины людей."},
	{id: "env.packs", group: GroupEnv, source: Org,
		name:  "Наборы окружения",
		desc:  "Администратор организации кладёт в каталог готовые наборы инструментов, хуков и правил из папки репозитория, с версиями.",
		needs: "Репозиторий с наборами.",
		why:   "Ставит на машины код из репозитория."},

	// Документы, макеты и ссылки
	{id: "HUB_DOCS", group: GroupDocs, source: Env,
		name:  "Документы и доски",
		desc:  "Агенты публикуют документы и доски, люди обсуждают их в комментариях и задают, кто видит и правит.",
		needs: "Отдельная служба документов (artifacts) со своим токеном; доски — ещё одна служба.",
		why:   "Нужна служба, которой нет в самом хабе.",
		on:    func(c *config.Config) bool { return c.Docs }},
	{id: "hub-board-events", group: GroupDocs, source: Rollout,
		name:  "Комментарии к доскам попадают в чат",
		desc:  "Хаб принимает комментарии к доскам, нарисованным агентом человека, и передаёт их в нужный чат.",
		needs: "Включённые «Документы и доски» со службой досок.",
		why:   "Нужна служба досок."},
	{id: "HUB_CHAT_GUEST_LINKS", group: GroupDocs, source: Env,
		name: "Публичные копии чатов",
		desc: "Автор выпускает неподписанную ссылку на очищенную копию чата, только для чтения. Ссылка живёт до семи дней и отзывается.",
		why:  "Открывает чужим людям содержимое чатов без входа.",
		on:   func(c *config.Config) bool { return c.ChatGuestLinks }},
	{id: "HUB_DESIGN_SNAPSHOT_LINKS", group: GroupDocs, source: Env,
		name: "Публичные снимки макетов",
		desc: "Автор макета выпускает ссылку на снимок страниц макета, только для чтения. Хаб отдаёт её, даже когда машина автора выключена.",
		why:  "Открывает макеты людям без входа.",
		on:   func(c *config.Config) bool { return c.DesignSnapshotLinks }},
	{id: "HUB_DESIGN_SHARE_DOCS", group: GroupDocs, source: Env,
		name:  "Ссылки на документы на адресе гостей",
		desc:  "Гостевые ссылки на документы открываются и на отдельном адресе для гостей макетов.",
		needs: "Отдельный адрес гостей (HUB_DESIGN_SHARE_URL) со своим доменом.",
		why:   "Нужен отдельный домен.",
		on:    func(c *config.Config) bool { return c.DesignShareDocs }},
	{id: "HUB_DESIGN_DERIVED", group: GroupDocs, source: Env,
		name: "Общий кэш макетов",
		desc: "То, что хаб считает из версии макета, хранится в базе и отдаётся из неё всеми копиями хаба.",
		why:  "Нужен только при нескольких копиях хаба: на одном сервере ничего не ускоряет.",
		on:   func(c *config.Config) bool { return c.DesignDerived }},

	// Данные и поиск на хабе
	{id: "hub-search", group: GroupHub, source: Rollout,
		name: "Поиск ⌘K из хаба",
		desc: "Поиск по чатам, макетам и проектам идёт по базе хаба и находит то, что лежит на выключенных машинах.",
		why:  "Ищет только по тому, что хаб уже знает: проект, которого в хранилище ещё нет, не найдётся, а к машине поиск не вернётся."},
	{id: "hub-projects", group: GroupHub, source: Rollout,
		name: "Проекты из хаба",
		desc: "Список проектов в боковой панели берётся из хаба: видны проекты и спящих машин, с пометкой «машина спит».",
		why:  "Список полон, только когда все машины передали хабу свои проекты; старые машины этого не делают."},
	{id: "hub-notices", group: GroupHub, source: Rollout,
		name: "Уведомления в хабе",
		desc: "Уведомления, что прочитано и настройки уведомлений живут в хабе: одни на все машины и устройства человека.",
		why:  "Уведомления, накопленные на машинах, в хаб не переносятся: они пропадут из вида."},
	{id: "hub-design-home", group: GroupHub, source: Rollout, defaultOn: true,
		name: "Главная макетов одним ответом",
		desc: "Страница макетов открывается быстрее: хаб собирает её сам и, если машина молчит, показывает копию библиотеки из хранилища. Проекты, которыми с вами поделились, появляются сразу, а не через полминуты. Если у хаба нет ответа, страница спрашивает машину, как раньше."},
	{id: "hub-apps-catalog", group: GroupHub, source: Rollout,
		name: "Каталог приложений из хаба",
		desc: "Типы документов приложений и сами документы берутся из базы хаба, а не с машины.",
		why:  "Документы видны только те, что записаны в хабе: пока он не хозяин данных о макетах, список неполон."},
	{id: "hub-eval", group: GroupHub, source: Rollout,
		name: "Проверки агентов по расписанию",
		desc: "Хаб сам ставит в очередь и запускает проверки качества агентов по расписанию человека.",
		why:  "Запускает работу агентов на машинах и тратит деньги: включается осознанно."},
	{id: "connections-shared", group: GroupHub, source: Rollout,
		name:  "Подключения человека на всех машинах",
		desc:  "Список подключений человека (без паролей и ключей) хранится в сейфе хаба, и подключение, сделанное на одной машине, появляется на остальных.",
		needs: "Сейф хаба (включённые «Секреты на хабе»)."},

	// Внешний вид
	{id: "ui.hx", group: GroupUI, source: Org,
		name: "Новый интерфейс хаба",
		desc: "Новый вид хаба и фоновая проверка сигналов организации.",
		why:  "В готовых установках (Docker Compose, Helm) он уже включён настройкой; хаб без неё стартует со старым видом."},
	{id: "ui.hx.live", group: GroupUI, source: Org,
		name:  "Живой хаб",
		desc:  "Анимация на всех экранах, свободные периоды, таблицы с сортировкой и экран «Инструменты».",
		needs: "«Эффективность» работает только с хостом кода.",
		why:   "Приносит экран «Эффективность», которому нужен хост кода."},
	{id: "org.events", group: GroupUI, source: Org, defaultOn: true,
		name: "Живые обновления экранов",
		desc: "Экраны сами обновляются, когда в организации меняются люди, приглашения и машины, и не требуют перезагрузки страницы."},

	// Секреты
	{id: "secrets.vault", group: GroupSafe, source: Org,
		name:  "Секреты на хабе",
		desc:  "Машины организации хранят секреты своих владельцев в сейфе хаба, а не в файлах на машине.",
		needs: "Сервер сейфа, совместимый с Vault (HUB_SECRETS_VAULT_ADDR).",
		why:   "Нужен отдельный сервер сейфа."},
}

// List is the screen's list for an organization. canChange says the viewer is an administrator of the installation:
// only then a flag or a switch may be toggled from the screen (the endpoints behind it ask the same).
func List(ctx context.Context, c *config.Config, g Gates, orgID string, canChange bool) []Item {
	out := make([]Item, 0, len(catalog))
	for _, d := range catalog {
		it := Item{ID: d.id, Group: d.group, Name: d.name, Description: d.desc, Source: d.source, DefaultOn: d.defaultOn, Needs: d.needs}
		if !d.defaultOn {
			it.Why = d.why
		}
		switch d.source {
		case Env:
			it.SetBy = d.id
			if d.on != nil {
				it.Enabled = d.on(c)
			} else {
				it.Enabled = true
			}
			if d.detail != nil {
				it.Detail = d.detail(c)
			}
		case Org:
			if g.Org != nil {
				it.Enabled = g.Org.On(ctx, orgID, d.id)
				it.Forced = g.Org.Forced(d.id)
			}
			if d.viaEnv != "" && slices.Contains(c.EnvFeatures, d.id) {
				it.Enabled, it.Forced, it.SetBy = true, true, d.viaEnv
			} else if it.Forced {
				it.SetBy = "HUB_FEATURES_FORCE"
			}
			it.CanToggle = canChange && !it.Forced
		case Rollout:
			if g.Rollout != nil {
				it.Enabled = g.Rollout.On(ctx, d.id, "", "")
			}
			it.CanToggle = canChange
		}
		out = append(out, it)
	}
	return out
}
