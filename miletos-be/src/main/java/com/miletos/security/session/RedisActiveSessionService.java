package com.miletos.security.session;

import com.miletos.config.JwtProperties;
import java.time.Duration;
import java.util.UUID;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Service;
import org.springframework.util.StringUtils;

@Service
public class RedisActiveSessionService implements ActiveSessionService {

    private final StringRedisTemplate redisTemplate;
    private final JwtProperties jwtProperties;
    private final String keyPrefix;

    public RedisActiveSessionService(
            StringRedisTemplate redisTemplate,
            JwtProperties jwtProperties,
            @Value("${miletos.security.session.key-prefix:miletos:auth:session}") String keyPrefix
    ) {
        this.redisTemplate = redisTemplate;
        this.jwtProperties = jwtProperties;
        this.keyPrefix = keyPrefix;
    }

    @Override
    public String createSession(Long userId) {
        String sessionId = UUID.randomUUID().toString();
        Duration ttl = Duration.ofMinutes(jwtProperties.accessTokenMinutes());

        if (ttl.isZero() || ttl.isNegative()) {
            throw new IllegalStateException("JWT access token TTL must be positive");
        }

        String userSessionKey = userSessionKey(userId);
        String previousSessionId = redisTemplate.opsForValue().get(userSessionKey);

        if (StringUtils.hasText(previousSessionId)) {
            redisTemplate.delete(sessionKey(previousSessionId));
        }

        redisTemplate.opsForValue().set(userSessionKey, sessionId, ttl);
        redisTemplate.opsForValue().set(sessionKey(sessionId), userId.toString(), ttl);

        return sessionId;
    }

    @Override
    public boolean isSessionActive(Long userId, String sessionId) {
        if (userId == null || !StringUtils.hasText(sessionId)) {
            return false;
        }

        String activeSessionId = redisTemplate.opsForValue().get(userSessionKey(userId));

        if (!sessionId.equals(activeSessionId)) {
            return false;
        }

        String sessionUserId = redisTemplate.opsForValue().get(sessionKey(sessionId));

        return userId.toString().equals(sessionUserId);
    }

    private String userSessionKey(Long userId) {
        return keyPrefix + ":user:" + userId;
    }

    private String sessionKey(String sessionId) {
        return keyPrefix + ":session:" + sessionId;
    }
}