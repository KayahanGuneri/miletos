package com.miletos.security.session;

import static org.assertj.core.api.Assertions.assertThat;

import com.miletos.testsupport.PostgreSqlContainerSupport;
import java.util.UUID;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.data.redis.core.StringRedisTemplate;

@SpringBootTest(properties = {
        "miletos.security.jwt.secret=test-session-secret-key-for-miletos-auth-flow-32-bytes-minimum",
        "miletos.security.jwt.issuer=https://miletos-session-test.local",
        "miletos.security.jwt.access-token-minutes=30"
})
class RedisActiveSessionServiceTest extends PostgreSqlContainerSupport {

    @Autowired
    private ActiveSessionService activeSessionService;

    @Autowired
    private StringRedisTemplate redisTemplate;

    @BeforeEach
    void clearRedis() {
        redisTemplate.getConnectionFactory()
                .getConnection()
                .serverCommands()
                .flushDb();
    }

    @Test
    void createsActiveSessionForUser() {
        Long userId = 10001L;

        String sessionId = activeSessionService.createSession(userId);

        assertThat(sessionId).isNotBlank();
        assertThat(activeSessionService.isSessionActive(userId, sessionId)).isTrue();
    }

    @Test
    void creatingNewSessionInvalidatesPreviousSessionForSameUser() {
        Long userId = 10002L;

        String firstSessionId = activeSessionService.createSession(userId);
        String secondSessionId = activeSessionService.createSession(userId);

        assertThat(secondSessionId).isNotBlank();
        assertThat(secondSessionId).isNotEqualTo(firstSessionId);
        assertThat(activeSessionService.isSessionActive(userId, firstSessionId)).isFalse();
        assertThat(activeSessionService.isSessionActive(userId, secondSessionId)).isTrue();
    }

    @Test
    void rejectsUnknownSession() {
        Long userId = 10003L;

        assertThat(activeSessionService.isSessionActive(userId, UUID.randomUUID().toString())).isFalse();
    }
}
