package secret

// Open answers this system's secret store for one HTML Clay profile.
func Open(profile string) (Store, error) {
	if err := checkName(profile); err != nil {
		return nil, err
	}
	return newCredman(profile), nil
}
