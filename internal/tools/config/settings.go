package config

import goconfig "github.com/artpar/pragma/internal/config"

type SettingDef = goconfig.SettingDef

var SupportedSettings = goconfig.SupportedSettings

func FindSetting(name string) *SettingDef {
	return goconfig.FindSetting(name)
}

func SettingNames() []string {
	return goconfig.SettingNames()
}
