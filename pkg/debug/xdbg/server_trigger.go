//go:build !windows

package xdbg

// watchTrigger 监听触发事件。
func (s *Server) watchTrigger() {
	defer s.wg.Done()

	eventCh := s.trigger.Watch(s.ctx)

	for {
		select {
		case <-s.ctx.Done():
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			s.handleTriggerEvent(event)
		}
	}
}

// handleTriggerEvent 处理触发事件。
func (s *Server) handleTriggerEvent(event TriggerEvent) {
	var err error
	switch event {
	case TriggerEventEnable:
		err = s.startListening()
	case TriggerEventDisable:
		err = s.stopListening()
	case TriggerEventToggle:
		if s.IsListening() {
			err = s.stopListening()
		} else {
			err = s.startListening()
		}
	}

	// 记录触发事件处理错误到审计日志
	if err != nil {
		s.audit(AuditEventCommandFailed, nil, "trigger:"+event.String(), nil, 0, err)
	}
}
