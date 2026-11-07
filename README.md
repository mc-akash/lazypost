# lazypost

A terminal HTTP client with the feel of lazygit: panels side by side, vim
navigation, single-key actions. Built for the daily loop of picking a
request, sending it and reading the response, switching environments and
pasting fresh tokens.

Requests, environments and variables are plain YAML files you can edit by
hand and keep in git. Secrets live in gitignored files and never reach the
history store.

## Install

From a checkout:

```sh
CGO_ENABLED=0 go build -o lazypost ./cmd/lazypost
```

| Flag | Meaning |
|---|---|
| `-w`, `-workspace <dir>` | workspace directory |
| `-insecure` | skip TLS certificate verification (also on when `insecure: true` is in the [config](#config)) |
| `-version` | print the version and exit |
| `-check-config` | check `config.yaml` (fields and keybinds) and exit |

## Quick start

1. Run `lazypost`. The workspace is created on first launch.
2. Press `N`, type a URL, press `enter`, then `s` to send. The response
   shows in `[5]`.
3. Press `w` to keep the request in a collection (e.g. `users/get user`).

Press `?` at any time for every key.

## Screen

```
[1] Environments  │ [4] Request    method + URL, tabs: Headers Body Auth Params
[2] Collections   │
[3] History       │ [5] Response   status, tabs: Body Headers Cookies
```

The focused panel has a green border. The bottom line lists the keys of
the focused panel: blue for actions, green for ones that create something,
red for ones that delete.

## Workspace

One central workspace holds all your APIs. Its location is, in order: the
`-w` flag, `$LAZYPOST_HOME`, `workspace` in the [config](#config), then
`~/lazypost`. lazypost never runs git; manage the folder yourself.

```
~/lazypost/
  lazypost.yaml            # optional: global variables
  secret.yaml              # optional, gitignored: secrets shared by all envs
  .gitignore               # always contains *.secret.yaml and /secret.yaml
  envs/
    staging.yaml           # environment variables (flat map)
    staging.secret.yaml    # gitignored: secrets of the staging env
  collections/
    users/                 # a folder; top-level folders group APIs
      _folder.yaml         # optional: folder variables and auth
      get-user.yaml        # one request per file
```

`lazypost.yaml` holds only workspace data that travels with it in git:

```yaml
globals:
  api_ver: v2
```

`timeout` and `insecure` live in the [config](#config). A `lazypost.yaml`
that still has them gets a startup warning; its globals still load. Remove
the old keys by hand to clear the warning.

`envs/staging.yaml` and `envs/staging.secret.yaml` are flat maps:

```yaml
base_url: https://stg.api.io
user_id: "42"
```

`collections/users/_folder.yaml`:

```yaml
vars:
  resource: users
auth:              # inherited by requests whose auth type is empty
  type: bearer
  token: "{{token}}"
```

`collections/users/create-user.yaml`:

```yaml
name: create user
method: POST
url: "{{base_url}}/{{api_ver}}/users"
params:
  - key: dry_run
    value: "true"
    disabled: true          # kept but not sent
headers:
  - key: Accept
    value: application/json
auth:
  type: bearer              # "" (inherit) | none | bearer | basic | apikey
  token: "{{token}}"
  # basic:  username, password
  # apikey: key, value, in: header | query
body:
  type: json                # none | json | form | multipart | raw
  content: |
    {"name": "Ada", "id": "{{$uuid}}"}
  # form:      form:  [{key, value, disabled}]
  # multipart: parts: [{key, value, file: true for a file path, disabled}]
```

### Unsaved request

A request doesn't need a collection until you want to keep it. `N` opens
the unsaved request in `[4]` (titled `[4] Request · unsaved`), even in an
empty workspace. Editing in `[4]` with nothing selected starts it too. It is
edited, sent and copied like any other request; folder auth doesn't apply.
`w` or `W` saves it into a collection.

There is one unsaved request. Selecting a Collections or History row shows
that row in `[4]` instead; `N` brings the unsaved request back as you left
it. Editing a History entry copies it into the unsaved request (asking
first if that would overwrite other unsaved work); the entry itself never
changes. The unsaved request survives restarts, and its sends appear in
History as `unsaved`.

### Drafts and local state

Edits are drafts, marked `●`, until saved with `w`. Quitting keeps drafts
and restores them on the next launch, together with the last environment
and request. A draft whose file changed on disk is marked `!`. Variable
edits save immediately.

History (the newest 200 sends, secrets redacted), drafts and the session
are stored in `$XDG_STATE_HOME/lazypost` (default `~/.local/state/lazypost`),
readable only by you.

## Variables

Use `{{name}}` in the URL, params, headers, auth fields and body. When a
name is defined in several places, the highest layer wins:

1. secrets of the active environment (`envs/<env>.secret.yaml`)
2. variables of the active environment (`envs/<env>.yaml`)
3. folder variables (`_folder.yaml`), the innermost folder first
4. shared secrets (`secret.yaml`)
5. globals (`lazypost.yaml`)

Built-ins: `{{$timestamp}}`, `{{$isoTimestamp}}`, `{{$guid}}`, `{{$uuid}}`,
`{{$randomInt}}`. Undefined variables are red in the request view and
`(missing)` in the variables table.

## Secrets

Saving a request (`w`, `W`) never writes a literal credential to its file:
the Authorization and Cookie headers, and the Auth tab's bearer token,
basic password and API key value, move into the active environment's
secrets and the file keeps a `{{reference}}`. Values that are already
references are left alone. With no active environment the save is refused.

cURL import applies the same rule. With no active environment, imported
credentials stay in the draft (or the unsaved request), which only the
local state holds.

## Keys

Names, values and confirmations open in a centred dialog: `enter` confirms,
`esc` cancels, `y`/`n` answer yes/no questions. The tables list the default
keys; every action can be rebound in the [config](#keybinds).

### Everywhere

| Key | Action |
|---|---|
| `1`-`5` | jump to panel |
| `0` | focus `[4]` / `[5]`, whichever was used last (from `[1]`: the variables table) |
| `tab` / `shift+tab` | next / previous panel |
| `h` / `l`, `←` / `→` | previous / next panel in the same column: `1`→`2`→`3` on the left, `4`↔`5` on the right |
| `j` / `k`, `g` / `G` | move down / up, top / bottom |
| `s` | send (resend from History) |
| `N` | open the unsaved request (in `[5]`: previous search match) |
| `E` | switch environment (`/` filters, `enter` activates) |
| `V` | variables of the active environment |
| `R` | reload the workspace from disk (drafts and selection are kept) |
| `esc` | cancel the running send; else back to the side panel |
| `?` | all keys |
| `q` / `ctrl+c` | quit (drafts are kept) |

### [1] Environments

| Key | Action |
|---|---|
| `space` | activate environment |
| `enter` | variables table |
| `n` / `d` | new / delete environment |

### Variables table

| Key | Action |
|---|---|
| `enter` | edit value |
| `p` | paste clipboard into value |
| `r` | reveal / hide secret |
| `a` / `d` | add / delete variable |
| `S` | move between vars and secrets |
| `esc` | back |

### [2] Collections

| Key | Action |
|---|---|
| `enter` | expand / collapse folder, open request in `[4]` |
| `n` | new request (end the name with `/` for a folder) |
| `d` | delete request / folder |
| `e` | edit request in `$EDITOR` |
| `w` | save changes to the request file |
| `W` | save as a new request (`folder/sub/name`, folders are created) |
| `c` | copy as cURL |
| `i` | import cURL |

### [3] History

| Key | Action |
|---|---|
| `enter` | open the entry's response in `[5]` |
| `s` | resend with current variables |

### [4] Request

| Key | Action |
|---|---|
| `[` / `]` | previous / next tab |
| `H` / `B` / `A` / `P` | Headers / Body / Auth / Params tab |
| `enter` | edit the row; on an empty tab add one; on Auth `Type:` pick the type; on Body open the body editor |
| `space` | send / don't send the row (`[x]` / `[ ]`) |
| `a` / `d` | add / delete row |
| `m` | pick method |
| `u` | edit URL |
| `t` | cycle body type |
| `f` | pretty-print the JSON body |
| `e` | edit the whole request in `$EDITOR` (also for form and multipart bodies) |
| `w` / `W` | save / save as |
| `c` / `i` | copy as cURL / import cURL into the unsaved request |

**Adding a header** (`a` on Headers) opens a picker: names already used in
the workspace first, then common ones. Type to filter; `↑`/`↓` move. `enter`
picks the highlighted name, or uses what you typed when nothing matches.
Common headers (`Accept`, `Content-Type`, ...) then offer common values the
same way.

**Body editor** (`enter` on the Body tab). Every key types into it except:

| Key | Action |
|---|---|
| `ctrl+s` | save and close (a body with no type becomes json if it parses, else raw) |
| `ctrl+f` | pretty-print the JSON (`{{var}}` templates are kept) |
| `ctrl+e` | continue in `$EDITOR`, then come back |
| `esc` | close (asks before dropping changes) |

### [5] Response

| Key | Action |
|---|---|
| `[` / `]` | previous / next tab (Body, Headers, Cookies) |
| `j` / `k`, `ctrl+d` / `ctrl+u` | scroll, half page |
| `/`, `n` / `N` | search, next / previous match |
| `y` | copy body |

## Config

Settings of the app itself, not of a workspace, live in
`$XDG_CONFIG_HOME/lazypost/config.yaml` (default
`~/.config/lazypost/config.yaml`). lazypost never creates it; without it
the defaults apply. Every key is optional:

```yaml
workspace: ~/lazypost   # default workspace; ~ expands to your home dir
timeout: 30s            # per-request timeout, must be > 0 (default 30s)
insecure: false         # skip TLS certificate verification
keybinds:               # action: key, or a list of keys
  send: ctrl+r
  next_item: [j, down, ctrl+n]
```

| Key | Precedence |
|---|---|
| `workspace` | the `-w` flag, then `$LAZYPOST_HOME`, then this key, then `~/lazypost` |
| `insecure` | on when this key or the `-insecure` flag is set |

The file is read strictly: an unknown key (a typo like `timout`), a
malformed or non-positive timeout, or broken YAML stops lazypost before
the UI opens, with an error naming the file and the problem. An empty file
is fine.

### Keybinds

`keybinds` maps an action to a key, or to a list of keys, written as
bubbletea names them: `x`, `F`, `ctrl+r`, `alt+x`, `home`. Both
`send: ctrl+r` and `send: [s, ctrl+r]` work. An override replaces the whole
default list, so `next_item: [x]` makes `j` and `down` stop moving. The
help screen (`?`) and the bottom line show the first key of each list.

| Action | Default | Where |
|---|---|---|
| `force_quit` | `ctrl+c` | everywhere, also in dialogs |
| `return` | `esc` | everywhere: cancel a send or dialog, go back, close |
| `confirm` | `enter` | everywhere: open, edit, confirm a dialog |
| `select` | `space` | `[1]` activate, `[4]` toggle row |
| `next_item` | `j`, `down` | everywhere |
| `prev_item` | `k`, `up` | everywhere |
| `prev_side_panel` | `h`, `left` | everywhere |
| `next_side_panel` | `l`, `right` | everywhere |
| `next_panel` | `tab` | everywhere |
| `prev_panel` | `shift+tab` | everywhere |
| `jump_to_panel` | `1`, `2`, `3`, `4`, `5` | everywhere; exactly five keys, for `[1]`-`[5]` in order |
| `main_view` | `0` | everywhere |
| `goto_top` | `g` | everywhere |
| `goto_bottom` | `G` | everywhere |
| `half_page_down` | `ctrl+d` | `[5]` |
| `half_page_up` | `ctrl+u` | `[5]` |
| `prev_tab` | `[` | `[4]`, `[5]` |
| `next_tab` | `]` | `[4]`, `[5]` |
| `body_save` | `ctrl+s` | body editor |
| `body_format` | `ctrl+f` | body editor |
| `body_external` | `ctrl+e` | body editor |
| `quit` | `q` | everywhere |
| `help` | `?` | everywhere |
| `send` | `s` | everywhere (resend in `[3]`) |
| `switch_env` | `E` | everywhere |
| `vars` | `V` | everywhere |
| `reload` | `R` | everywhere |
| `new_unsaved` | `N` | everywhere except `[5]` |
| `new` | `n` | `[1]`, `[2]` |
| `delete` | `d` | `[1]`, `[2]`, `[4]`, variables table |
| `add` | `a` | `[4]`, variables table |
| `edit_in_editor` | `e` | `[2]`, `[4]` |
| `save` | `w` | `[2]`, `[4]` |
| `save_as` | `W` | `[2]`, `[4]` |
| `copy_curl` | `c` | `[2]`, `[4]` |
| `import_curl` | `i` | `[2]`, `[4]` |
| `method` | `m` | `[4]` |
| `edit_url` | `u` | `[4]` |
| `body_type` | `t` | `[4]` |
| `format_body` | `f` | `[4]` |
| `headers_tab` | `H` | `[4]` |
| `body_tab` | `B` | `[4]` |
| `auth_tab` | `A` | `[4]` |
| `params_tab` | `P` | `[4]` |
| `search` | `/` | `[5]`, and filtering in the environment picker |
| `next_match` | `n` | `[5]` |
| `prev_match` | `N` | `[5]` |
| `copy_body` | `y` | `[5]` |
| `paste` | `p` | variables table |
| `reveal` | `r` | variables table |
| `toggle_secret` | `S` | variables table |

While you type (dialogs, filtering pickers, the header picker, the
body editor) a single character is text, so only the non-character keys of
a list work there: with `next_item: [x, down]`, `x` is typed and `down`
moves. That is why `force_quit`, `return`, `confirm`, `body_save`,
`body_format` and `body_external` can't use a character key.

A key is a single character, a name (`enter`, `tab`, `esc`, `space`,
`backspace`, `up`, `down`, `left`, `right`, `home`, `end`, `pgup`, `pgdown`,
`insert`, `delete`, `f1`…`f12`), or either with `ctrl+`, `alt+` or `shift+`
in front, written in lower case (`ctrl+r`, `shift+tab`).

lazypost stops with an error naming the action for an unknown action, an
empty list or key, an unknown key, a key listed twice, `jump_to_panel`
without exactly five keys, or two actions sharing a key where both apply.

Not rebindable: typing itself, `backspace`, `y`/`n` in yes/no questions and
`ctrl+n`/`ctrl+p` in the header picker. Careful with `return` and
`confirm`: a bad bind can make dialogs hard to leave; `ctrl+c`
(`force_quit`) still quits unless you rebind it too.

Two actions may share a key only when they never work in the same panel,
as `new` and `next_match` do by default. An unknown action, an empty or unknown
key, or a clash stops lazypost at startup, naming the action and the key.

## cURL

`c` copies the request (its draft if it has one) as a one-line cURL command
with variables resolved and auth folded into headers. Secrets are included;
it only goes to your clipboard.

`i` asks for a cURL command (prefilled from the clipboard), then a name,
and saves a new request in the current folder. In `[4]`, or with no
collections yet, it fills the unsaved request instead, asking first if one
exists. Multi-line commands with `\` line breaks and combined flags like
`-sSL` work; `-d @file` is refused, since lazypost does not read the file.
Credentials follow the rule in [Secrets](#secrets).

## Clipboard

Copy and paste use the system clipboard: built in on macOS and Windows; on
Linux, `wl-clipboard` (Wayland) or `xclip` / `xsel` (X11).
Copies are also sent with OSC 52, so they work over SSH. Without a
clipboard tool, `p` in the variables table opens an input that accepts the
terminal's own paste.
