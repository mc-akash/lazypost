package config

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"
)

type Keys []string

func (k *Keys) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*k = Keys{n.Value}
		return nil
	}
	var list []string
	if err := n.Decode(&list); err != nil {
		return err
	}
	*k = list
	return nil
}

type Keymap struct {
	ForceQuit     Keys `yaml:"force_quit"      default:"ctrl+c"     scope:"all" typing:"true"`
	Return        Keys `yaml:"return"          default:"esc"        scope:"all" typing:"true"`
	Confirm       Keys `yaml:"confirm"         default:"enter"      scope:"all" typing:"true"`
	Select        Keys `yaml:"select"          default:"space"      scope:"all"`
	NextItem      Keys `yaml:"next_item"       default:"j down"     scope:"all"`
	PrevItem      Keys `yaml:"prev_item"       default:"k up"       scope:"all"`
	PrevSidePanel Keys `yaml:"prev_side_panel" default:"h left"     scope:"all"`
	NextSidePanel Keys `yaml:"next_side_panel" default:"l right"    scope:"all"`
	NextPanel     Keys `yaml:"next_panel"      default:"tab"        scope:"all"`
	PrevPanel     Keys `yaml:"prev_panel"      default:"shift+tab"  scope:"all"`
	JumpToPanel   Keys `yaml:"jump_to_panel"   default:"1 2 3 4 5"  scope:"all"`
	MainView      Keys `yaml:"main_view"       default:"0"          scope:"all"`
	GotoTop       Keys `yaml:"goto_top"        default:"g"          scope:"all"`
	GotoBottom    Keys `yaml:"goto_bottom"     default:"G"          scope:"all"`
	HalfPageDown  Keys `yaml:"half_page_down"  default:"ctrl+d"     scope:"response"`
	HalfPageUp    Keys `yaml:"half_page_up"    default:"ctrl+u"     scope:"response"`
	PrevTab       Keys `yaml:"prev_tab"        default:"["          scope:"request,response"`
	NextTab       Keys `yaml:"next_tab"        default:"]"          scope:"request,response"`
	BodySave      Keys `yaml:"body_save"       default:"ctrl+s"     scope:"request" typing:"true"`
	BodyFormat    Keys `yaml:"body_format"     default:"ctrl+f"     scope:"request" typing:"true"`
	BodyExternal  Keys `yaml:"body_external"   default:"ctrl+e"     scope:"request" typing:"true"`
	Quit          Keys `yaml:"quit"            default:"q"          scope:"all"`
	Help          Keys `yaml:"help"            default:"?"          scope:"all"`
	Send          Keys `yaml:"send"            default:"s"          scope:"all"`
	SwitchEnv     Keys `yaml:"switch_env"      default:"E"          scope:"all"`
	Vars          Keys `yaml:"vars"            default:"V"          scope:"all"`
	Reload        Keys `yaml:"reload"          default:"R"          scope:"all"`
	NewUnsaved    Keys `yaml:"new_unsaved"     default:"N"          scope:"envs,collections,history,request,vars"`
	New           Keys `yaml:"new"             default:"n"          scope:"envs,collections"`
	Delete        Keys `yaml:"delete"          default:"d"          scope:"envs,collections,request,vars"`
	Add           Keys `yaml:"add"             default:"a"          scope:"request,vars"`
	EditInEditor  Keys `yaml:"edit_in_editor"  default:"e"          scope:"collections,request"`
	Save          Keys `yaml:"save"            default:"w"          scope:"collections,request"`
	SaveAs        Keys `yaml:"save_as"         default:"W"          scope:"collections,request"`
	CopyCurl      Keys `yaml:"copy_curl"       default:"c"          scope:"collections,request"`
	ImportCurl    Keys `yaml:"import_curl"     default:"i"          scope:"collections,request"`
	Method        Keys `yaml:"method"          default:"m"          scope:"request"`
	EditURL       Keys `yaml:"edit_url"        default:"u"          scope:"request"`
	BodyType      Keys `yaml:"body_type"       default:"t"          scope:"request"`
	FormatBody    Keys `yaml:"format_body"     default:"f"          scope:"request"`
	HeadersTab    Keys `yaml:"headers_tab"     default:"H"          scope:"request"`
	BodyTab       Keys `yaml:"body_tab"        default:"B"          scope:"request"`
	AuthTab       Keys `yaml:"auth_tab"        default:"A"          scope:"request"`
	ParamsTab     Keys `yaml:"params_tab"      default:"P"          scope:"request"`
	Search        Keys `yaml:"search"          default:"/"          scope:"response"`
	NextMatch     Keys `yaml:"next_match"      default:"n"          scope:"response"`
	PrevMatch     Keys `yaml:"prev_match"      default:"N"          scope:"response"`
	CopyBody      Keys `yaml:"copy_body"       default:"y"          scope:"response"`
	Paste         Keys `yaml:"paste"           default:"p"          scope:"vars"`
	Reveal        Keys `yaml:"reveal"          default:"r"          scope:"vars"`
	ToggleSecret  Keys `yaml:"toggle_secret"   default:"S"          scope:"vars"`
}

func DefaultKeymap() Keymap {
	var k Keymap
	v := reflect.ValueOf(&k).Elem()
	for i := range v.NumField() {
		v.Field(i).Set(reflect.ValueOf(Keys(strings.Split(v.Type().Field(i).Tag.Get("default"), " "))))
	}
	return k
}

const jumpKeys = 5

var keyName = regexp.MustCompile(`^(ctrl\+)?(alt\+)?(shift\+)?(meta\+)?(hyper\+)?(super\+)?(enter|tab|backspace|esc|space|up|down|left|right|home|end|pgup|pgdown|insert|delete|f[0-9]{1,2}|[^\s\p{Lu}])$`)

type action struct {
	name   string
	keys   Keys
	scope  []string
	typing bool
}

func (k Keymap) actions() []action {
	v := reflect.ValueOf(k)
	acts := make([]action, v.NumField())
	for i := range acts {
		tag := v.Type().Field(i).Tag
		acts[i] = action{name: tag.Get("yaml"), keys: v.Field(i).Interface().(Keys), scope: strings.Split(tag.Get("scope"), ","), typing: tag.Get("typing") == "true"}
	}
	return acts
}

func (k Keymap) Validate() error {
	acts := k.actions()
	for _, a := range acts {
		if err := a.check(); err != nil {
			return fmt.Errorf("keybinds: %s: %w", a.name, err)
		}
	}
	return clashes(acts)
}

func (a action) check() error {
	switch {
	case len(a.keys) == 0:
		return errors.New("empty key list")
	case a.name == "jump_to_panel" && len(a.keys) != jumpKeys:
		return fmt.Errorf("needs %d keys, got %d", jumpKeys, len(a.keys))
	}
	for i, k := range a.keys {
		switch {
		case k == "":
			return errors.New("empty key")
		case slices.Contains(a.keys[:i], k):
			return fmt.Errorf("duplicate key %q", k)
		case !keyName.MatchString(k) && (!IsText(k) || k == " "):
			return fmt.Errorf("unknown key %q", k)
		case a.typing && IsText(k):
			return fmt.Errorf("%q is printable, but this action must work while typing", k)
		}
	}
	return nil
}

func (a action) overlaps(b action) bool {
	return slices.Contains(a.scope, "all") || slices.Contains(b.scope, "all") ||
		slices.ContainsFunc(a.scope, func(s string) bool { return slices.Contains(b.scope, s) })
}

func clashes(acts []action) error {
	type bound struct {
		a   action
		key string
	}
	var all []bound
	for _, a := range acts {
		for _, k := range a.keys {
			all = append(all, bound{a, k})
		}
	}
	for i, x := range all {
		for _, y := range all[i+1:] {
			if x.key == y.key && x.a.overlaps(y.a) {
				return fmt.Errorf("keybinds: %s and %s both use %q", x.a.name, y.a.name, x.key)
			}
		}
	}
	return nil
}

func IsText(key string) bool {
	r := []rune(key)
	return len(r) == 1 && unicode.IsPrint(r[0])
}
