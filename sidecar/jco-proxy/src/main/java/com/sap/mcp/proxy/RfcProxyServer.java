package com.sap.mcp.proxy;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.sap.conn.jco.JCoException;
import com.sap.mcp.proxy.config.ConnectionConfig;
import com.sap.mcp.proxy.model.ProxyRequest;
import com.sap.mcp.proxy.model.ProxyResponse;
import com.sap.mcp.proxy.model.RfcCallRequest;
import com.sap.mcp.proxy.model.RfcCallResponse;
import io.javalin.Javalin;
import io.javalin.http.Context;
import io.javalin.json.JsonMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.lang.reflect.Type;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * HTTP server that proxies requests to SAP via RFC.
 * This is the main entry point for the JCo proxy sidecar.
 *
 * Usage:
 *   java -cp jco-proxy.jar:sapjco3.jar com.sap.mcp.proxy.RfcProxyServer \
 *     --port 8081 \
 *     --mshost bd1ffccs.sap.corpintra.net \
 *     --msserv 3601 \
 *     --r3name BD1 \
 *     --group GUI \
 *     --client 010 \
 *     --user USERNAME \
 *     --password PASSWORD
 */
public class RfcProxyServer {
    private static final Logger logger = LoggerFactory.getLogger(RfcProxyServer.class);
    private static final Gson gson = new GsonBuilder().create();

    private final int port;
    private final ConnectionConfig config;
    private final JCoConnectionManager connectionManager;
    private final StatefulSessionManager sessionManager;
    private final RestRfcEndpointCaller rfcCaller;
    private final DirectRfcCaller directRfcCaller;
    private Javalin app;

    public static void main(String[] args) {
        logger.info("Starting RFC Proxy Server...");

        try {
            int port = parsePort(args);
            ConnectionConfig config = buildConfig(args);
            config.validate();

            logger.info("Configuration: {}", config);

            RfcProxyServer server = new RfcProxyServer(port, config);
            server.start();

            // Register shutdown hook
            Runtime.getRuntime().addShutdownHook(new Thread(() -> {
                logger.info("Shutdown signal received");
                server.stop();
            }));

        } catch (Exception e) {
            logger.error("Failed to start server: {}", e.getMessage(), e);
            // Also print to stderr so the Go sidecar manager can capture the error
            System.err.println("SIDECAR_ERROR: " + e.getMessage());
            System.exit(1);
        }
    }

    public RfcProxyServer(int port, ConnectionConfig config) {
        this.port = port;
        this.config = config;
        this.connectionManager = new JCoConnectionManager(config);
        this.sessionManager = new StatefulSessionManager();
        this.rfcCaller = new RestRfcEndpointCaller(connectionManager);
        this.directRfcCaller = new DirectRfcCaller(connectionManager);
    }

    /**
     * Start the HTTP server and initialize JCo connection.
     */
    public void start() throws JCoException {
        // Initialize JCo connection first
        connectionManager.initialize();

        // Create custom JSON mapper using Gson
        JsonMapper gsonMapper = new JsonMapper() {
            @Override
            public String toJsonString(Object obj, Type type) {
                return gson.toJson(obj, type);
            }

            @Override
            public <T> T fromJsonString(String json, Type type) {
                return gson.fromJson(json, type);
            }
        };

        // Create Javalin app with Gson JSON mapper
        this.app = Javalin.create(javalinConfig -> {
            javalinConfig.showJavalinBanner = false;
            javalinConfig.jsonMapper(gsonMapper);
        });

        // Routes
        app.post("/rfc-proxy", this::handleProxyRequest);
        app.post("/rfc-call", this::handleDirectRfc);
        app.get("/health", this::handleHealthCheck);
        app.get("/rfc-pool/status", this::handlePoolStatus);
        app.post("/rfc-pool/terminate", this::handleTerminate);

        // Error handling
        app.exception(Exception.class, (e, ctx) -> {
            logger.error("Unhandled exception: {}", e.getMessage(), e);
            ctx.status(500);
            ctx.json(ProxyResponse.error(500, "Internal server error: " + e.getMessage()));
        });

        // Start server (port 0 = dynamic assignment)
        app.start(port);

        // Get actual bound port (important for dynamic port assignment)
        int actualPort = app.port();

        // Output actual port in a parseable format for Node.js sidecar manager
        // This MUST be printed to stdout for the parent process to discover the port
        System.out.println("SIDECAR_PORT=" + actualPort);
        System.out.flush();

        logger.info("RFC Proxy Server started on port {}", actualPort);
        logger.info("Health check: http://localhost:{}/health", actualPort);
        logger.info("Proxy endpoint: http://localhost:{}/rfc-proxy", actualPort);
    }

    /**
     * Stop the server and close connections.
     */
    public void stop() {
        logger.info("Stopping RFC Proxy Server...");
        if (app != null) {
            app.stop();
        }
        sessionManager.shutdown();
        connectionManager.close();
        logger.info("RFC Proxy Server stopped");
    }

    /**
     * Handle proxy requests - main endpoint.
     */
    private void handleProxyRequest(Context ctx) {
        try {
            ProxyRequest request = gson.fromJson(ctx.body(), ProxyRequest.class);

            if (request == null || request.getMethod() == null || request.getUri() == null) {
                ctx.status(400);
                ctx.json(ProxyResponse.error(400, "Invalid request: method and uri are required"));
                return;
            }

            logger.debug("Received proxy request: {}", request);

            // Check if this is a stateful session request
            String sessionType = request.getHeaders().get("X-sap-adt-sessiontype");
            boolean isStateful = "stateful".equalsIgnoreCase(sessionType);
            logger.info("[PROXY] Request: {} {} | sessionType={}", request.getMethod(), request.getUri().substring(0, Math.min(80, request.getUri().length())), sessionType);

            String sessionId = null;
            if (isStateful) {
                // Extract existing session ID from Cookie header
                String cookieHeader = request.getHeaders().get("Cookie");
                logger.info("[PROXY] Cookie header received: '{}'", cookieHeader);
                sessionId = extractSessionId(cookieHeader);
                logger.info("[PROXY] Extracted sessionId from cookie: '{}'", sessionId);

                // Begin or reuse session
                String returnedSessionId = sessionManager.beginSession(connectionManager.getDestination(), sessionId);
                logger.info("[PROXY] beginSession returned: '{}' (passed in: '{}')", returnedSessionId, sessionId);
                sessionId = returnedSessionId;
            }

            // Inject SAP's stored cookies into the request before executing
            // This is critical for session continuity (debugger, locks, etc.)
            if (isStateful && sessionId != null && sessionManager.isStatefulSession(sessionId)) {
                injectSapCookies(request, sessionId);
            }

            // Execute the RFC call on the session's dedicated thread (for thread affinity)
            final String finalSessionId = sessionId;
            ProxyResponse response = sessionManager.executeInSession(sessionId, () -> {
                logger.info("[PROXY] Executing RFC call on thread: {} (session: {})",
                    Thread.currentThread().getName(), finalSessionId);
                return rfcCaller.execute(request);
            });

            // Store SAP's cookies from the response in the session
            if (isStateful && sessionId != null) {
                storeSapCookies(response, sessionId);

                // Return the sidecar session ID as vsp-session-id cookie
                // (not sap-contextid, to avoid overwriting SAP's own cookie)
                String cookieValue = "sap-contextid=" + sessionId + "; Path=/; HttpOnly";
                response.getHeaders().put("Set-Cookie", cookieValue);
                logger.debug("Set sidecar session cookie: {}", sessionId);
            }

            logger.debug("Returning response: {} {}", response.getStatusCode(), response.getReasonPhrase());
            ctx.json(response);

        } catch (Exception e) {
            logger.error("Error handling proxy request: {}", e.getMessage(), e);
            ctx.status(500);
            ctx.json(ProxyResponse.error(500, "Proxy error: " + e.getMessage()));
        }
    }

    /**
     * Inject SAP's stored cookies into the request headers before sending to SADT_REST_RFC_ENDPOINT.
     * This ensures SAP-level session continuity (critical for debugger attach after listen).
     */
    private void injectSapCookies(ProxyRequest request, String sessionId) {
        // Get SAP cookies stored from previous responses
        String sapCookies = sessionManager.getSapCookies(sessionId);
        if (sapCookies != null) {
            // Replace or set the Cookie header with SAP's cookies
            request.getHeaders().put("Cookie", sapCookies);
            logger.info("[PROXY] Injected SAP cookies into request: {}", sapCookies);
        }
    }

    /**
     * Store SAP's cookies from the response in the session for future requests.
     * SADT_REST_RFC_ENDPOINT returns Set-Cookie headers just like a normal HTTP response.
     */
    private void storeSapCookies(ProxyResponse response, String sessionId) {
        // Check for Set-Cookie in the SAP response (from SADT_REST_RFC_ENDPOINT)
        String setCookie = response.getHeaders().get("Set-Cookie");
        if (setCookie == null) setCookie = response.getHeaders().get("set-cookie");
        if (setCookie != null && !setCookie.isEmpty()) {
            sessionManager.storeSapCookies(sessionId, setCookie);
            logger.info("[PROXY] Stored SAP cookies from response: {}", setCookie);
        }
    }

    /**
     * Extract session ID from Cookie header.
     * Looks for "sap-contextid=..." cookie.
     */
    private String extractSessionId(String cookieHeader) {
        if (cookieHeader == null || cookieHeader.isEmpty()) {
            return null;
        }

        // Parse cookies (format: "name1=value1; name2=value2")
        String[] cookies = cookieHeader.split(";");
        for (String cookie : cookies) {
            String[] parts = cookie.trim().split("=", 2);
            if (parts.length == 2 && "sap-contextid".equals(parts[0])) {
                return parts[1];
            }
        }
        return null;
    }

    /**
     * Handle direct RFC function module calls.
     */
    private void handleDirectRfc(Context ctx) {
        try {
            RfcCallRequest request = gson.fromJson(ctx.body(), RfcCallRequest.class);

            if (request == null || request.getFunction() == null || request.getFunction().isEmpty()) {
                ctx.status(400);
                ctx.json(RfcCallResponse.error("Invalid request: function name is required"));
                return;
            }

            logger.info("[RFC-CALL] Function: {}", request.getFunction());

            RfcCallResponse response = directRfcCaller.execute(request);

            if (response.getError() != null) {
                ctx.status(500);
            } else {
                ctx.status(200);
            }
            ctx.json(response);

        } catch (Exception e) {
            logger.error("Error handling RFC call: {}", e.getMessage(), e);
            ctx.status(500);
            ctx.json(RfcCallResponse.error("RFC call error: " + e.getMessage()));
        }
    }

    /**
     * Handle health check requests.
     */
    private void handleHealthCheck(Context ctx) {
        if (connectionManager.isHealthy()) {
            ctx.status(200);
            ctx.result("OK");
        } else {
            ctx.status(503);
            ctx.result("SAP connection unhealthy");
        }
    }

    /**
     * Handle RFC pool status requests.
     * Returns information about active stateful sessions and JCo pool configuration.
     * This is used by the GetRfcPoolStatus MCP tool.
     */
    private void handlePoolStatus(Context ctx) {
        try {
            // Get session info from StatefulSessionManager
            Map<String, Object> sessionInfo = sessionManager.getSessionInfo();

            // JCo pool config - these are the default JCo values (informational only).
            // JCo doesn't expose runtime pool metrics. If you customize jco.pool_capacity or
            // jco.peak_limit in your destination configuration, update these values to match.
            Map<String, Object> jcoConfig = new HashMap<>();
            jcoConfig.put("poolCapacity", 5);   // Default: jco.pool_capacity
            jcoConfig.put("peakLimit", 10);     // Default: jco.peak_limit

            // Build response
            Map<String, Object> response = new HashMap<>();
            response.put("sessions", sessionInfo.get("sessions"));
            response.put("sessionTimeout", sessionInfo.get("timeout"));
            response.put("activeSessionCount", sessionInfo.get("count"));
            response.put("jcoPoolConfig", jcoConfig);

            ctx.status(200);
            ctx.json(response);

        } catch (Exception e) {
            logger.error("Error getting pool status: {}", e.getMessage(), e);
            ctx.status(500);
            ctx.json(ProxyResponse.error(500, "Error getting pool status: " + e.getMessage()));
        }
    }

    /**
     * Handle RFC pool session termination requests.
     * Terminates sessions based on criteria: specific session ID, age threshold, or force all.
     * This is used by the TerminateRfcConnection MCP tool.
     */
    private void handleTerminate(Context ctx) {
        try {
            // Parse request body
            @SuppressWarnings("unchecked")
            Map<String, Object> requestBody = gson.fromJson(ctx.body(), Map.class);

            String sessionId = (String) requestBody.get("sessionId");
            Integer ageThresholdSeconds = null;
            Boolean force = null;

            // Handle ageThresholdSeconds (could be Integer or Double from JSON)
            Object ageObj = requestBody.get("ageThresholdSeconds");
            if (ageObj instanceof Number) {
                ageThresholdSeconds = ((Number) ageObj).intValue();
            }

            // Handle force flag
            Object forceObj = requestBody.get("force");
            if (forceObj instanceof Boolean) {
                force = (Boolean) forceObj;
            }

            logger.info("Terminate request - sessionId: {}, ageThreshold: {}s, force: {}",
                sessionId, ageThresholdSeconds, force);

            // Terminate sessions
            List<Map<String, Object>> terminatedSessions =
                sessionManager.terminateSessions(sessionId, ageThresholdSeconds, force);

            // Build response
            java.util.Map<String, Object> response = new java.util.HashMap<>();
            response.put("terminatedCount", terminatedSessions.size());
            response.put("sessions", terminatedSessions);

            ctx.status(200);
            ctx.json(response);

        } catch (Exception e) {
            logger.error("Error terminating sessions: {}", e.getMessage(), e);
            ctx.status(500);
            ctx.json(ProxyResponse.error(500, "Error terminating sessions: " + e.getMessage()));
        }
    }

    /**
     * Parse port from command-line arguments.
     */
    private static int parsePort(String[] args) {
        for (int i = 0; i < args.length - 1; i++) {
            if ("--port".equals(args[i])) {
                try {
                    return Integer.parseInt(args[i + 1]);
                } catch (NumberFormatException e) {
                    logger.warn("Invalid port value '{}', using default", args[i + 1]);
                }
            }
        }
        // Default port from environment or fallback
        String envPort = System.getenv("SAP_RFC_PROXY_PORT");
        if (envPort != null) {
            try {
                return Integer.parseInt(envPort);
            } catch (NumberFormatException e) {
                logger.warn("Invalid SAP_RFC_PROXY_PORT value '{}', using default", envPort);
            }
        }
        return 8081;
    }

    /**
     * Build configuration from environment and command-line args.
     */
    private static ConnectionConfig buildConfig(String[] args) {
        // Start with environment config
        ConnectionConfig envConfig = ConnectionConfig.fromEnvironment();

        // Overlay command-line args
        ConnectionConfig argsConfig = ConnectionConfig.fromArgs(args);

        // Merge (args take precedence)
        return ConnectionConfig.merge(envConfig, argsConfig);
    }
}
