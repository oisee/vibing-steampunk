package com.sap.mcp.proxy.model;

import java.util.HashMap;
import java.util.Map;

/**
 * Represents an incoming HTTP request to be proxied via RFC.
 */
public class ProxyRequest {
    private String method;
    private String uri;
    private Map<String, String> headers;
    private String body;

    public ProxyRequest() {
        this.headers = new HashMap<>();
    }

    public ProxyRequest(String method, String uri, Map<String, String> headers, String body) {
        this.method = method;
        this.uri = uri;
        this.headers = headers != null ? headers : new HashMap<>();
        this.body = body;
    }

    public String getMethod() {
        return method;
    }

    public void setMethod(String method) {
        this.method = method;
    }

    public String getUri() {
        return uri;
    }

    public void setUri(String uri) {
        this.uri = uri;
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
        return "ProxyRequest{" +
                "method='" + method + '\'' +
                ", uri='" + uri + '\'' +
                ", headers=" + headers.size() + " entries" +
                ", body=" + (body != null ? body.length() + " chars" : "null") +
                '}';
    }
}
