package com.sap.mcp.proxy.model;

import java.util.HashMap;
import java.util.Map;

/**
 * Request model for direct RFC function module calls via /rfc-call endpoint.
 */
public class RfcCallRequest {
    private String function;
    private Map<String, Object> params;

    public RfcCallRequest() {
        this.params = new HashMap<>();
    }

    public String getFunction() {
        return function;
    }

    public void setFunction(String function) {
        this.function = function;
    }

    public Map<String, Object> getParams() {
        return params;
    }

    public void setParams(Map<String, Object> params) {
        this.params = params != null ? params : new HashMap<>();
    }

    @Override
    public String toString() {
        return "RfcCallRequest{function='" + function + "', params=" + params.size() + " entries}";
    }
}
