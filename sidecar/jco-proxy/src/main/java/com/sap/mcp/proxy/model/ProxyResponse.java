package com.sap.mcp.proxy.model;

import java.util.HashMap;
import java.util.Map;

/**
 * Represents an HTTP response returned from the RFC call.
 */
public class ProxyResponse {
    private int statusCode;
    private String reasonPhrase;
    private Map<String, String> headers;
    private String body;

    public ProxyResponse() {
        this.headers = new HashMap<>();
    }

    public ProxyResponse(int statusCode, String reasonPhrase, Map<String, String> headers, String body) {
        this.statusCode = statusCode;
        this.reasonPhrase = reasonPhrase;
        this.headers = headers != null ? headers : new HashMap<>();
        this.body = body;
    }

    /**
     * Create an error response.
     */
    public static ProxyResponse error(int statusCode, String message) {
        return new ProxyResponse(statusCode, message, new HashMap<>(), message);
    }

    public int getStatusCode() {
        return statusCode;
    }

    public void setStatusCode(int statusCode) {
        this.statusCode = statusCode;
    }

    public String getReasonPhrase() {
        return reasonPhrase;
    }

    public void setReasonPhrase(String reasonPhrase) {
        this.reasonPhrase = reasonPhrase;
    }

    public Map<String, String> getHeaders() {
        return headers;
    }

    public void setHeaders(Map<String, String> headers) {
        this.headers = headers;
    }

    public String getBody() {
        return body;
    }

    public void setBody(String body) {
        this.body = body;
    }

    @Override
    public String toString() {
        return "ProxyResponse{" +
                "statusCode=" + statusCode +
                ", reasonPhrase='" + reasonPhrase + '\'' +
                ", headers=" + headers.size() + " entries" +
                ", body=" + (body != null ? body.length() + " chars" : "null") +
                '}';
    }
}
