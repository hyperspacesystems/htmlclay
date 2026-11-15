package secret

// Open answers this system's secret store for one HTML Clay profile. configDir
// is that profile's config folder, where a file store keeps its files.
func Open(profile, configDir string) (Store, error) {
	if err := checkName(profile); err != nil {
		return nil, err
	}
	return newKeychain("com.htmlclay.sync." + profile), nil
}
