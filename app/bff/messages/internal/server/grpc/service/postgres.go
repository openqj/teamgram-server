package service

func (s *Service) ClosePostgres() error {
	if s != nil && s.svcCtx != nil && s.svcCtx.Dao != nil {
		s.svcCtx.Dao.Close()
	}
	return nil
}
