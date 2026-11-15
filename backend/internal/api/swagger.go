package api

// Swagger UI loaded from a CDN; it reads the embedded spec at /openapi.yaml.
const swaggerHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>PRD Assistant API</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css"></head>
<body><div id="ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
<script>window.onload=()=>SwaggerUIBundle({url:'/openapi.yaml',dom_id:'#ui'})</script>
</body></html>`
