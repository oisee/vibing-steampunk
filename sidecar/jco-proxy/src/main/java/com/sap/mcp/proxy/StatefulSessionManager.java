package com.sap.mcp.proxy;

import com.sap.conn.jco.JCoContext;
import com.sap.conn.jco.JCoDestination;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.Date;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.Callable;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;

/**
 * Manages stateful RFC sessions using JCoContext with thread affinity.
 *
 * CRITICAL: JCoContext is thread-local. To maintain session state across
 * multiple HTTP requests (LOCK → PUT → UNLOCK), all RFC calls for a session
 * must execute on the SAME thread.
 *
 * This manager creates a single-threaded executor for each session, ensuring
 * JCoContext.begin() and all subsequent RFC calls run on the same thread.
 */
public class StatefulSessionManager {
    private static final Logger logger = LoggerFactory.getLogger(StatefulSessionManager.class);
    private static final long SESSION_TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes

    private final Map<String, SessionInfo> sessions = new ConcurrentHashMap<>();
    private final ScheduledExecutorService cleanupExecutor = Executors.newSingleThreadScheduledExecutor();

    /**
     * Session info with dedicated executor for thread affinity.
     */
    static class SessionInfo {
        final JCoDestination destination;
        final ExecutorService executor; // Single-threaded executor for this session
        final long creationTime;
        volatile long lastAccessTime;
        volatile boolean contextActive;

        // SAP-level cookies from SADT_REST_RFC_ENDPOINT responses.
        // These must be forwarded on subsequent requests to maintain SAP's internal
        // session state (e.g., debugger sessions, ADT locks).
        // Key = cookie name (e.g., "sap-contextid"), Value = full cookie string
        private final Map<String, String> sapCookies = new ConcurrentHashMap<>();

        SessionInfo(JCoDestination destination) {
            this.destination = destination;
            // Create a single-threaded executor - all RFC calls for this session run here
            this.executor = Executors.newSingleThreadExecutor(r -> {
                Thread t = new Thread(r, "JCoSession-" + UUID.randomUUID().toString().substring(0, 8));
                t.setDaemon(true);
                return t;
            });
            this.creationTime = System.currentTimeMillis();
            this.lastAccessTime = System.currentTimeMillis();
            this.contextActive = false;
        }

        /**
         * Store SAP cookies from a Set-Cookie response header.
         * Parses "name=value; Path=/; ..." format and stores name→value.
         */
        void storeSapCookie(String setCookieHeader) {
            if (setCookieHeader == null || setCookieHeader.isEmpty()) return;
            // Parse "name=value; attributes..."
            String nameValue = setCookieHeader.split(";")[0].trim();
            int eq = nameValue.indexOf('=');
            if (eq > 0) {
                String name = nameValue.substring(0, eq);
                String value = nameValue.substring(eq + 1);
                sapCookies.put(name, value);
            }
        }

        /**
         * Get SAP cookies as a Cookie header value ("name1=value1; name2=value2").
         */
        String getSapCookieHeader() {
            if (sapCookies.isEmpty()) return null;
            StringBuilder sb = new StringBuilder();
            for (Map.Entry<String, String> entry : sapCookies.entrySet()) {
                if (sb.length() > 0) sb.append("; ");
                sb.append(entry.getKey()).append("=").append(entry.getValue());
            }
            return sb.toString();
        }

        void touch() {
            this.lastAccessTime = System.currentTimeMillis();
        }

        boolean isExpired() {
            return System.currentTimeMillis() - lastAccessTime > SESSION_TIMEOUT_MS;
        }

        long getCreationTime() {
            return creationTime;
        }

        long getLastAccessTime() {
            return lastAccessTime;
        }

        boolean isContextActive() {
            return contextActive;
        }

        void shutdown() {
            executor.shutdown();
            try {
                if (!executor.awaitTermination(5, TimeUnit.SECONDS)) {
                    executor.shutdownNow();
                }
            } catch (InterruptedException e) {
                executor.shutdownNow();
                Thread.currentThread().interrupt();
            }
        }
    }

    public StatefulSessionManager() {
        // Start cleanup task to remove expired sessions
        cleanupExecutor.scheduleAtFixedRate(this::cleanupExpiredSessions,
            1, 1, TimeUnit.MINUTES);
    }

    /**
     * Begin a stateful session or reuse existing one.
     * Returns a session ID that should be sent back to the client as a cookie.
     *
     * For NEW sessions, JCoContext.begin() is called on the session's dedicated thread.
     * For REUSED sessions, the existing JCoContext is already active - no new begin() call is made.
     */
    public String beginSession(JCoDestination destination, String existingSessionId) {
        logger.info("[SESSION] beginSession called with existingSessionId: {}", existingSessionId);
        logger.info("[SESSION] Current sessions map size: {}, keys: {}", sessions.size(), sessions.keySet());

        // Check if we have an existing valid session
        if (existingSessionId != null) {
            synchronized (sessions) {
                SessionInfo session = sessions.get(existingSessionId);
                logger.info("[SESSION] Lookup result for '{}': session={}, expired={}",
                    existingSessionId, (session != null ? "FOUND" : "NULL"),
                    (session != null ? session.isExpired() : "N/A"));

                if (session != null && !session.isExpired()) {
                    session.touch();
                    logger.info("[SESSION] REUSING existing session: {} (contextActive={})",
                        existingSessionId, session.contextActive);
                    return existingSessionId;
                } else if (session != null) {
                    // Session expired, clean it up
                    logger.info("[SESSION] Session {} expired, ending it", existingSessionId);
                    endSession(existingSessionId);
                } else {
                    logger.warn("[SESSION] Session {} NOT FOUND in map! Cannot reuse.", existingSessionId);
                }
            }
        }

        // Create new session with dedicated executor
        String sessionId = UUID.randomUUID().toString();
        SessionInfo session = new SessionInfo(destination);
        logger.info("[SESSION] Creating NEW session: {} (existingSessionId was: {})", sessionId, existingSessionId);

        // Begin JCoContext on the session's dedicated thread
        try {
            Future<Void> future = session.executor.submit(() -> {
                logger.info("[SESSION] Starting JCoContext.begin() on thread: {}", Thread.currentThread().getName());
                JCoContext.begin(destination);
                logger.info("[SESSION] JCoContext.begin() completed on thread: {}", Thread.currentThread().getName());
                return null;
            });
            future.get(30, TimeUnit.SECONDS); // Wait for JCoContext.begin() to complete
            session.contextActive = true;
            logger.info("[SESSION] Started stateful RFC session: {} with JCoContext.begin()", sessionId);
        } catch (Exception e) {
            logger.error("[SESSION] Failed to begin JCoContext for session {}: {}", sessionId, e.getMessage());
            session.shutdown();
            throw new RuntimeException("Failed to start stateful session", e);
        }

        // Only add to map after successful initialization
        sessions.put(sessionId, session);
        logger.info("[SESSION] Added session {} to map. New map size: {}", sessionId, sessions.size());
        return sessionId;
    }

    /**
     * Execute an RFC call within a session's thread context.
     * This ensures JCoContext thread affinity is maintained.
     *
     * @param sessionId The session ID (can be null for stateless calls)
     * @param rfcCall The RFC call to execute
     * @param <T> Return type
     * @return The result of the RFC call
     */
    public <T> T executeInSession(String sessionId, Callable<T> rfcCall) throws Exception {
        if (sessionId == null) {
            // No session - execute directly on current thread
            logger.debug("[SESSION] No session, executing RFC call on current thread");
            return rfcCall.call();
        }

        SessionInfo session = sessions.get(sessionId);
        if (session == null || session.isExpired()) {
            logger.warn("[SESSION] Session {} not found or expired, executing on current thread", sessionId);
            return rfcCall.call();
        }

        session.touch();
        logger.info("[SESSION] Executing RFC call in session {} on dedicated thread", sessionId);

        // Execute the RFC call on the session's dedicated thread
        Future<T> future = session.executor.submit(rfcCall);
        return future.get(300, TimeUnit.SECONDS); // 5 minute timeout (supports debugger long-polling up to 240s)
    }

    /**
     * End a stateful session and close the JCoContext.
     */
    public void endSession(String sessionId) {
        if (sessionId == null) {
            return;
        }

        SessionInfo session = sessions.remove(sessionId);
        if (session != null) {
            if (session.contextActive) {
                try {
                    // End JCoContext on the session's dedicated thread
                    Future<Void> future = session.executor.submit(() -> {
                        logger.info("[SESSION] Ending JCoContext on thread: {}", Thread.currentThread().getName());
                        JCoContext.end(session.destination);
                        return null;
                    });
                    future.get(10, TimeUnit.SECONDS);
                    logger.info("Ended stateful RFC session: {}", sessionId);
                } catch (Exception e) {
                    logger.error("Error ending JCoContext for session {}: {}", sessionId, e.getMessage());
                }
            }
            session.shutdown();
        }
    }

    /**
     * Store SAP cookies from a Set-Cookie response header into the session.
     */
    public void storeSapCookies(String sessionId, String setCookieHeader) {
        if (sessionId == null) return;
        SessionInfo session = sessions.get(sessionId);
        if (session != null) {
            session.storeSapCookie(setCookieHeader);
            logger.info("[SESSION] Stored SAP cookie for session {}: {}", sessionId, setCookieHeader);
        }
    }

    /**
     * Get SAP cookies stored in the session as a Cookie header value.
     */
    public String getSapCookies(String sessionId) {
        if (sessionId == null) return null;
        SessionInfo session = sessions.get(sessionId);
        if (session != null) {
            return session.getSapCookieHeader();
        }
        return null;
    }

    /**
     * Check if a session is stateful (has active context).
     */
    public boolean isStatefulSession(String sessionId) {
        if (sessionId == null) {
            return false;
        }
        SessionInfo session = sessions.get(sessionId);
        return session != null && session.contextActive && !session.isExpired();
    }

    /**
     * Cleanup expired sessions periodically.
     */
    private void cleanupExpiredSessions() {
        try {
            List<String> expiredSessionIds = new ArrayList<>();

            // First, identify expired sessions
            for (Map.Entry<String, SessionInfo> entry : sessions.entrySet()) {
                if (entry.getValue().isExpired()) {
                    expiredSessionIds.add(entry.getKey());
                }
            }

            // Then, end each expired session
            for (String sessionId : expiredSessionIds) {
                logger.info("Cleaning up expired session: {}", sessionId);
                endSession(sessionId);
            }
        } catch (Exception e) {
            logger.error("Error during session cleanup: {}", e.getMessage(), e);
        }
    }

    /**
     * Terminate sessions based on criteria.
     */
    public List<Map<String, Object>> terminateSessions(
            String targetSessionId,
            Integer ageThresholdSeconds,
            Boolean force
    ) {
        List<Map<String, Object>> terminatedSessions = new ArrayList<>();
        long now = System.currentTimeMillis();

        logger.info("Terminating sessions - targetSessionId: {}, ageThreshold: {}s, force: {}",
            targetSessionId, ageThresholdSeconds, force);

        List<String> sessionsToTerminate = new ArrayList<>();

        // Use ConcurrentHashMap's weakly consistent iterator - no synchronization needed
        // for read-only iteration. Actual termination happens outside this loop.
        for (Map.Entry<String, SessionInfo> entry : sessions.entrySet()) {
            String sessionId = entry.getKey();
            SessionInfo info = entry.getValue();
            boolean shouldTerminate = false;

            if (Boolean.TRUE.equals(force)) {
                shouldTerminate = true;
            } else if (targetSessionId != null && sessionId.equals(targetSessionId)) {
                shouldTerminate = true;
            } else if (ageThresholdSeconds != null) {
                long ageSeconds = (now - info.getCreationTime()) / 1000;
                if (ageSeconds >= ageThresholdSeconds) {
                    shouldTerminate = true;
                }
            }

            if (shouldTerminate) {
                sessionsToTerminate.add(sessionId);

                Map<String, Object> sessionData = new HashMap<>();
                sessionData.put("sessionId", sessionId);
                sessionData.put("ageSeconds", (now - info.getCreationTime()) / 1000);
                sessionData.put("idleSeconds", (now - info.getLastAccessTime()) / 1000);
                sessionData.put("contextActive", info.isContextActive());
                terminatedSessions.add(sessionData);
            }
        }

        // End sessions outside the synchronized block
        for (String sessionId : sessionsToTerminate) {
            try {
                endSession(sessionId);
                // Update terminated status
                for (Map<String, Object> data : terminatedSessions) {
                    if (sessionId.equals(data.get("sessionId"))) {
                        data.put("terminated", true);
                        break;
                    }
                }
            } catch (Exception e) {
                logger.error("Error terminating session {}: {}", sessionId, e.getMessage());
            }
        }

        logger.info("Terminated {} session(s)", terminatedSessions.size());
        return terminatedSessions;
    }

    /**
     * Shutdown the session manager and clean up all active sessions.
     */
    public void shutdown() {
        logger.info("Shutting down StatefulSessionManager");
        cleanupExecutor.shutdown();

        // End all active sessions
        List<String> allSessions = new ArrayList<>(sessions.keySet());
        for (String sessionId : allSessions) {
            endSession(sessionId);
        }
    }

    /**
     * Get detailed information about all active sessions.
     */
    public Map<String, Object> getSessionInfo() {
        List<Map<String, Object>> sessionList = new ArrayList<>();
        long now = System.currentTimeMillis();

        // Use ConcurrentHashMap's weakly consistent iterator - no synchronization needed
        // for read-only monitoring operations
        for (Map.Entry<String, SessionInfo> entry : sessions.entrySet()) {
            String sessionId = entry.getKey();
            SessionInfo info = entry.getValue();

            Map<String, Object> sessionData = new HashMap<>();
            sessionData.put("sessionId", sessionId);
            sessionData.put("createdAt", new Date(info.getCreationTime()));
            sessionData.put("lastAccessedAt", new Date(info.getLastAccessTime()));
            sessionData.put("ageSeconds", (now - info.getCreationTime()) / 1000);
            sessionData.put("idleSeconds", (now - info.getLastAccessTime()) / 1000);
            sessionData.put("contextActive", info.isContextActive());

            sessionList.add(sessionData);
        }

        Map<String, Object> result = new HashMap<>();
        result.put("sessions", sessionList);
        result.put("timeout", SESSION_TIMEOUT_MS / 1000);
        result.put("count", sessions.size());

        return result;
    }

    /**
     * Get the number of currently active sessions.
     */
    public int getActiveSessionCount() {
        return sessions.size();
    }
}
