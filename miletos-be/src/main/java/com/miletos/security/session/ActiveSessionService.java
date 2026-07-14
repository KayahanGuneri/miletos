package com.miletos.security.session;


public interface ActiveSessionService {

    String createSession(Long userId);

    boolean isSessionActive(Long userId, String sessionId);
}