package yale

func (m *Yale) Run() error {
	var err error
	err = m.RotateKeys()
	if err != nil {
		return err
	}
	err = m.DisableKeys()
	if err != nil {
		return err
	}
	err = m.DeleteKeys()
	if err != nil {
		return err
	}
	err = m.PopulateCache()
	if err != nil {
		return err
	}
	return nil
}
