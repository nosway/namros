# S3 API Swagger View

This view renders the
[implementation-based S3 compatibility reference](s3-api-compatibility-reference.md)
from the [OpenAPI JSON](../api/namros-s3.openapi.json).

<div class="note" markdown="1">

**View only.** S3 selects logical operations by query subresources and headers,
while OpenAPI permits one operation for each path and HTTP method. The support
matrix below is authoritative for logical operations. The Swagger paths show
the three physical dispatch resources. Submit buttons are disabled because
Swagger UI does not generate AWS SigV4 signatures and cannot portably express
slash-containing greedy object keys.

</div>

The raw specification remains available offline. This rendered page loads the
pinned Swagger UI assets from jsDelivr and therefore needs browser access to
that CDN.

<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.32.11/swagger-ui.css">

<style>
  #namros-s3-matrix-status { margin: 1rem 0; }
  #namros-s3-matrix { border-collapse: collapse; display: block; max-height: 36rem; overflow: auto; width: 100%; }
  #namros-s3-matrix th, #namros-s3-matrix td { border: .05rem solid var(--md-default-fg-color--lightest); padding: .45rem .6rem; text-align: left; vertical-align: top; }
  #namros-s3-matrix th { background: var(--md-default-bg-color); position: sticky; top: 0; z-index: 1; }
  #namros-s3-matrix code { white-space: nowrap; }
  #swagger-ui { background: #fff; margin-top: 2rem; padding: .5rem; }
</style>

## Logical Operation Matrix

<p id="namros-s3-matrix-status">Loading the operation catalog…</p>
<table id="namros-s3-matrix" hidden>
  <thead>
    <tr>
      <th>Operation</th>
      <th>Method</th>
      <th>Resource</th>
      <th>Selector</th>
      <th>Status</th>
      <th>Notes</th>
    </tr>
  </thead>
  <tbody></tbody>
</table>

## Physical Dispatcher View

<div id="swagger-ui"></div>

<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.32.11/swagger-ui-bundle.js"></script>
<script>
(() => {
  const specUrl = new URL("../../api/namros-s3.openapi.json", window.location.href).toString();
  const status = document.getElementById("namros-s3-matrix-status");
  const table = document.getElementById("namros-s3-matrix");
  const body = table.querySelector("tbody");

  const cell = (row, value, code = false) => {
    const td = document.createElement("td");
    const child = code ? document.createElement("code") : document.createTextNode(value || "");
    if (code) child.textContent = value || "";
    td.appendChild(child);
    row.appendChild(td);
  };

  fetch(specUrl)
    .then(response => {
      if (!response.ok) throw new Error(`OpenAPI fetch returned ${response.status}`);
      return response.json();
    })
    .then(spec => {
      const operations = spec["x-namros-operation-matrix"] || [];
      operations.forEach(operation => {
        const row = document.createElement("tr");
        cell(row, operation.operationId, true);
        cell(row, operation.method, true);
        cell(row, operation.resource, true);
        cell(row, operation.selector, true);
        cell(row, operation.status);
        cell(row, operation.notes);
        body.appendChild(row);
      });
      status.textContent = `${operations.length} logical S3 operations are cataloged.`;
      table.hidden = false;
    })
    .catch(error => {
      status.textContent = `Could not load the operation catalog: ${error.message}`;
    });

  if (window.SwaggerUIBundle) {
    window.SwaggerUIBundle({
      url: specUrl,
      dom_id: "#swagger-ui",
      deepLinking: true,
      displayOperationId: true,
      docExpansion: "none",
      filter: true,
      supportedSubmitMethods: [],
      validatorUrl: null,
      presets: [window.SwaggerUIBundle.presets.apis]
    });
  } else {
    document.getElementById("swagger-ui").textContent = "Swagger UI assets could not be loaded.";
  }
})();
</script>
