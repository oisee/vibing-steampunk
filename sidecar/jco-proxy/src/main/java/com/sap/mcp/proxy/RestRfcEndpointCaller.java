package com.sap.mcp.proxy;

import com.sap.conn.jco.JCoDestination;
import com.sap.conn.jco.JCoException;
import com.sap.conn.jco.JCoFunction;
import com.sap.conn.jco.JCoStructure;
import com.sap.conn.jco.JCoTable;
import com.sap.mcp.proxy.model.ProxyRequest;
import com.sap.mcp.proxy.model.ProxyResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;

/**
 * Calls the SAP function module SADT_REST_RFC_ENDPOINT to execute HTTP requests via RFC.
 * This is the same mechanism used by Eclipse ADT to communicate with SAP when HTTP ports are blocked.
 */
public class RestRfcEndpointCaller {
    private static final Logger logger = LoggerFactory.getLogger(RestRfcEndpointCaller.class);
    private static final String FUNCTION_NAME = "SADT_REST_RFC_ENDPOINT";

    private final JCoConnectionManager connectionManager;

    public RestRfcEndpointCaller(JCoConnectionManager connectionManager) {
        this.connectionManager = connectionManager;
    }

    /**
     * Execute an HTTP request via RFC.
     *
     * @param request The HTTP request to execute
     * @return The HTTP response from SAP
     */
    public ProxyResponse execute(ProxyRequest request) {
        logger.info("Executing RFC call: {} {}", request.getMethod(), request.getUri());

        try {
            JCoDestination destination = connectionManager.getDestination();
            JCoFunction function = destination.getRepository().getFunction(FUNCTION_NAME);

            if (function == null) {
                String error = "Function " + FUNCTION_NAME + " not found in SAP. " +
                        "Ensure ADT services are active (transaction SICF).";
                logger.error(error);
                return ProxyResponse.error(500, error);
            }

            // Populate REQUEST parameter
            JCoStructure requestStruct = function.getImportParameterList().getStructure("REQUEST");
            populateRequest(requestStruct, request);

            // Execute RFC call
            long startTime = System.currentTimeMillis();
            function.execute(destination);
            long duration = System.currentTimeMillis() - startTime;
            logger.debug("RFC call completed in {}ms", duration);

            // Extract RESPONSE
            JCoStructure responseStruct = function.getExportParameterList().getStructure("RESPONSE");
            return extractResponse(responseStruct);

        } catch (JCoException e) {
            String error = "RFC call failed: " + e.getMessage();
            logger.error(error, e);
            return ProxyResponse.error(502, error);
        } catch (Exception e) {
            String error = "Unexpected error: " + e.getMessage();
            logger.error(error, e);
            return ProxyResponse.error(500, error);
        }
    }

    /**
     * Populate the REQUEST structure with HTTP request data.
     */
    private void populateRequest(JCoStructure requestStruct, ProxyRequest request) {
        // REQUEST_LINE sub-structure
        JCoStructure requestLine = requestStruct.getStructure("REQUEST_LINE");
        requestLine.setValue("METHOD", request.getMethod());
        requestLine.setValue("URI", request.getUri());
        requestLine.setValue("VERSION", "HTTP/1.1");

        logger.debug("Request line: {} {} HTTP/1.1", request.getMethod(), request.getUri());

        // HEADER_FIELDS table
        JCoTable headerFields = requestStruct.getTable("HEADER_FIELDS");

        // Add headers from request
        for (Map.Entry<String, String> header : request.getHeaders().entrySet()) {
            addHeader(headerFields, header.getKey(), header.getValue());
        }

        // Ensure required headers are present
        ensureHeader(headerFields, request.getHeaders(), "User-Agent", "MCP-ABAP-ADT/1.0");
        ensureHeader(headerFields, request.getHeaders(), "Accept", "*/*");

        logger.debug("Added {} header fields", headerFields.getNumRows());

        // MESSAGE_BODY (if present)
        if (request.getBody() != null && !request.getBody().isEmpty()) {
            byte[] bodyBytes = request.getBody().getBytes(StandardCharsets.UTF_8);
            requestStruct.setValue("MESSAGE_BODY", bodyBytes);
            logger.debug("Added message body: {} bytes", bodyBytes.length);
        }
    }

    /**
     * Extract the RESPONSE structure into a ProxyResponse.
     */
    private ProxyResponse extractResponse(JCoStructure responseStruct) {
        // STATUS_LINE
        JCoStructure statusLine = responseStruct.getStructure("STATUS_LINE");
        // STATUS_CODE is returned as STRING from SAP, need to parse it
        String statusCodeStr = statusLine.getString("STATUS_CODE").trim();
        int statusCode;
        try {
            statusCode = Integer.parseInt(statusCodeStr);
        } catch (NumberFormatException e) {
            logger.warn("Failed to parse status code '{}', defaulting to 500", statusCodeStr);
            statusCode = 500;
        }
        String reasonPhrase = statusLine.getString("REASON_PHRASE");

        logger.debug("Response status: {} {}", statusCode, reasonPhrase);

        // HEADER_FIELDS
        Map<String, String> responseHeaders = new HashMap<>();
        JCoTable respHeaderFields = responseStruct.getTable("HEADER_FIELDS");
        for (int i = 0; i < respHeaderFields.getNumRows(); i++) {
            respHeaderFields.setRow(i);
            String name = respHeaderFields.getString("NAME");
            String value = respHeaderFields.getString("VALUE");
            responseHeaders.put(name, value);
        }

        logger.debug("Response has {} headers", responseHeaders.size());

        // MESSAGE_BODY
        byte[] bodyBytes = responseStruct.getByteArray("MESSAGE_BODY");
        String body = "";
        if (bodyBytes != null && bodyBytes.length > 0) {
            body = new String(bodyBytes, StandardCharsets.UTF_8);
            logger.debug("Response body: {} bytes", body.length());
        }

        return new ProxyResponse(statusCode, reasonPhrase, responseHeaders, body);
    }

    /**
     * Add a header to the header fields table.
     */
    private void addHeader(JCoTable table, String name, String value) {
        table.appendRow();
        table.setValue("NAME", name);
        table.setValue("VALUE", value);
    }

    /**
     * Ensure a header exists, adding default if not present.
     */
    private void ensureHeader(JCoTable table, Map<String, String> existingHeaders, String name, String defaultValue) {
        // Check if header already exists (case-insensitive)
        for (String key : existingHeaders.keySet()) {
            if (key.equalsIgnoreCase(name)) {
                return; // Already present
            }
        }
        // Add default
        addHeader(table, name, defaultValue);
    }
}
