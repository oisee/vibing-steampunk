package com.sap.mcp.proxy;

import com.sap.conn.jco.JCoDestination;
import com.sap.conn.jco.JCoDestinationManager;
import com.sap.conn.jco.JCoException;
import com.sap.conn.jco.ext.DestinationDataEventListener;
import com.sap.conn.jco.ext.DestinationDataProvider;
import com.sap.conn.jco.ext.Environment;
import com.sap.mcp.proxy.config.ConnectionConfig;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Properties;

/**
 * Manages JCo destination connections to SAP systems.
 * Supports both direct application server and message server (load balancing) connections.
 * Implements DestinationDataProvider to provide connection properties dynamically.
 */
public class JCoConnectionManager implements DestinationDataProvider {
    private static final Logger logger = LoggerFactory.getLogger(JCoConnectionManager.class);
    private static final String DESTINATION_NAME = "MCP_ADT_DESTINATION";

    private final ConnectionConfig config;
    private JCoDestination destination;
    private boolean registered = false;

    public JCoConnectionManager(ConnectionConfig config) {
        this.config = config;
    }

    /**
     * Initialize the JCo connection.
     * Registers this as destination provider and tests connectivity.
     */
    public synchronized void initialize() throws JCoException {
        if (config.isDirectConnection()) {
            logger.info("Initializing JCo connection (direct) to {}:{}", config.getAsHost(), config.getSysnr());
        } else {
            logger.info("Initializing JCo connection (load balancing) to {} ({})", config.getR3Name(), config.getMsHost());
        }

        // Register as destination provider if not already
        if (!registered) {
            try {
                Environment.registerDestinationDataProvider(this);
                registered = true;
                logger.debug("Registered as DestinationDataProvider");
            } catch (IllegalStateException e) {
                // Already registered - this can happen on restart
                logger.warn("DestinationDataProvider already registered: {}", e.getMessage());
            }
        }

        // Get destination (triggers getDestinationProperties)
        this.destination = JCoDestinationManager.getDestination(DESTINATION_NAME);

        // Test connection with ping
        logger.info("Testing connection...");
        destination.ping();
        if (config.isDirectConnection()) {
            logger.info("JCo connection established successfully to {}:{}", config.getAsHost(), config.getSysnr());
        } else {
            logger.info("JCo connection established successfully to {}", config.getR3Name());
        }

        // Log connection attributes
        logger.debug("Connection attributes: {}", destination.getAttributes());
    }

    /**
     * Get the JCo destination for making RFC calls.
     */
    public JCoDestination getDestination() {
        if (destination == null) {
            throw new IllegalStateException("JCo connection not initialized. Call initialize() first.");
        }
        return destination;
    }

    /**
     * Check if connection is healthy.
     */
    public boolean isHealthy() {
        if (destination == null) return false;
        try {
            destination.ping();
            return true;
        } catch (JCoException e) {
            logger.warn("Connection health check failed: {}", e.getMessage());
            return false;
        }
    }

    /**
     * Close the connection and unregister provider.
     */
    public synchronized void close() {
        logger.info("Closing JCo connection");
        if (registered) {
            try {
                Environment.unregisterDestinationDataProvider(this);
                registered = false;
            } catch (IllegalStateException e) {
                logger.warn("Error unregistering provider: {}", e.getMessage());
            }
        }
        destination = null;
    }

    // DestinationDataProvider interface implementation

    @Override
    public Properties getDestinationProperties(String destinationName) {
        if (!DESTINATION_NAME.equals(destinationName)) {
            return null;
        }

        Properties props = new Properties();

        // Connection type - direct app server or message server (load balancing)
        if (config.isDirectConnection()) {
            // Direct application server connection
            props.setProperty(DestinationDataProvider.JCO_ASHOST, config.getAsHost());
            props.setProperty(DestinationDataProvider.JCO_SYSNR, config.getSysnr());
            logger.debug("Using direct connection: {}:{}", config.getAsHost(), config.getSysnr());
        } else {
            // Message server connection (load balancing)
            props.setProperty(DestinationDataProvider.JCO_MSHOST, config.getMsHost());
            props.setProperty(DestinationDataProvider.JCO_MSSERV, config.getMsServ());
            props.setProperty(DestinationDataProvider.JCO_R3NAME, config.getR3Name());
            props.setProperty(DestinationDataProvider.JCO_GROUP, config.getGroup());
            logger.debug("Using load balancing: {} ({})", config.getMsHost(), config.getR3Name());
        }

        // Authentication
        props.setProperty(DestinationDataProvider.JCO_CLIENT, config.getClient());
        props.setProperty(DestinationDataProvider.JCO_USER, config.getUsername());
        props.setProperty(DestinationDataProvider.JCO_PASSWD, config.getPassword());
        props.setProperty(DestinationDataProvider.JCO_LANG, config.getLanguage());

        // Connection pool settings
        props.setProperty(DestinationDataProvider.JCO_POOL_CAPACITY, "5");
        props.setProperty(DestinationDataProvider.JCO_PEAK_LIMIT, "10");

        logger.debug("Providing destination properties for {}", destinationName);
        return props;
    }

    @Override
    public void setDestinationDataEventListener(DestinationDataEventListener listener) {
        // We don't support runtime configuration changes
    }

    @Override
    public boolean supportsEvents() {
        return false;
    }
}
