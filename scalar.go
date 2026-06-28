package main

import _ "embed"

//go:embed scalar.bundle.js
var scalarBundle []byte

var scalarHTML = []byte(`<!doctype html>
<html>
<head>
	<title>dynserver API</title>
	<meta charset="utf-8"/>
	<meta name="viewport" content="width=device-width, initial-scale=1"/>
</head>
<body>
	<script id="api-reference" data-url="/openapi.yaml"></script>
	<script src="/scalar.bundle.js"></script>
</body>
</html>`)
