package service

func (s *Service) ClosePostgres() error {
	if s == nil || s.svcCtx == nil || s.svcCtx.Dao == nil {
		return nil
	}
	return s.svcCtx.Dao.Close()
}
