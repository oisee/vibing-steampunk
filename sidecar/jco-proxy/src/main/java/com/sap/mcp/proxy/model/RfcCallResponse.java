package com.sap.mcp.proxy.model;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Response model for direct RFC function module calls via /rfc-call endpoint.
 */
public class RfcCallResponse {
    private Map<String, Object> exports;
    private Map<String, List<Map<String, Object>>> tables;
    private long durationMs;
    private String error;

    public RfcCallResponse() {
        this.exports = new HashMap<>();
        this.tables = new HashMap<>();
    }

    /**
     * Create a successful response.
     */
    public static RfcCallResponse success(Map<String, Object> exports,
                                           Map<String, List<Map<String, Object>>> tables,
                                           long durationMs) {
        RfcCallResponse resp = new RfcCallResponse();
        resp.exports = exports != null ? exports : new HashMap<>();
        resp.tables = tables != null ? tables : new HashMap<>();
        resp.durationMs = durationMs;
        return resp;
    }

    /**
     * Create an error response.
     */
    public static RfcCallResponse error(String message) {
        RfcCallResponse resp = new RfcCallResponse();
        resp.error = message;
        return resp;
    }

    public Map<String, Object> getExports() {
        return exports;
    }

    public void setExports(Map<String, Object> exports) {
        this.exports = exports;
    }

    public Map<String, List<Map<String, Object>>> getTables() {
        return tables;
    }

    public void setTables(Map<String, List<Map<String, Object>>> tables) {
        this.tables = tables;
    }

    public long getDurationMs() {
        return durationMs;
    }

    public void setDurationMs(long durationMs) {
        this.durationMs = durationMs;
    }

    public String getError() {
        return error;
    }

    public void setError(String error) {
        this.error = error;
    }
}
