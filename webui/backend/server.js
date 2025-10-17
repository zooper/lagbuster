const express = require('express');
const { createProxyMiddleware } = require('http-proxy-middleware');
const path = require('path');

const app = express();
const PORT = process.env.PORT || 3000;
const LAGBUSTER_API = process.env.LAGBUSTER_API || 'http://127.0.0.1:8080';

// Proxy API requests to lagbuster backend
app.use('/api', createProxyMiddleware({
  target: LAGBUSTER_API,
  changeOrigin: true,
  onError: (err, req, res) => {
    console.error('Proxy error:', err);
    res.status(502).json({
      error: 'Unable to connect to Lagbuster API',
      details: err.message
    });
  }
}));

// Proxy WebSocket connections
app.use('/ws', createProxyMiddleware({
  target: LAGBUSTER_API,
  ws: true,
  changeOrigin: true,
  onError: (err, req, res) => {
    console.error('WebSocket proxy error:', err);
  }
}));

// Serve static files from React build in production
const buildPath = path.join(__dirname, '../frontend/build');
app.use(express.static(buildPath));

// All remaining requests return the React app (SPA routing)
app.get('*', (req, res) => {
  res.sendFile(path.join(buildPath, 'index.html'));
});

app.listen(PORT, () => {
  console.log(`Lagbuster Web UI running on http://localhost:${PORT}`);
  console.log(`Proxying API requests to ${LAGBUSTER_API}`);
});
