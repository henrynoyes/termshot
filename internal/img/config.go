package img

import (
	_ "embed"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Factor          float64 `yaml:"factor"`
	FontSize        float64 `yaml:"font_size"`
	FontDPI         float64 `yaml:"font_dpi"`
	FontDir         string  `yaml:"font_dir"`
	Margin          float64 `yaml:"margin"`
	Padding         float64 `yaml:"padding"`
	DrawDecorations bool    `yaml:"draw_decorations"`
	DrawShadow      bool    `yaml:"draw_shadow"`
	ShadowBaseColor string  `yaml:"shadow_base_color"`
	ShadowRadius    float64 `yaml:"shadow_radius"`
	ShadowOffsetX   float64 `yaml:"shadow_offset_x"`
	ShadowOffsetY   float64 `yaml:"shadow_offset_y"`
	LineSpacing     float64 `yaml:"line_spacing"`
	TabSpaces       int     `yaml:"tab_spaces"`

	Prompt          string `yaml:"prompt"`
	PromptColor     string `yaml:"prompt_color"`
	CommandColor    string `yaml:"command_color"`
	OutlineColor    string `yaml:"outline_color"`
	BackgroundColor string `yaml:"background_color"`
}

//go:embed config.yaml
var configYAML []byte

var config Config

func init() {
	if err := yaml.Unmarshal(configYAML, &config); err != nil {
		panic("failed to parse internal/img/config.yaml: " + err.Error())
	}
}
