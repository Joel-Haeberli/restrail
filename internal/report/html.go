package report

import (
	"encoding/json"
	"fmt"
	"html"
	"restrail/internal/openapi"
	"restrail/internal/runner"
	"strings"
	"time"
)

type HTMLReporter struct{}

func (r *HTMLReporter) Generate(result *runner.RunResult) ([]byte, error) {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Restrail Test Report</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 2rem; color: #333; background: #fafafa; }
  h1 { color: #1a1a2e; border-bottom: 2px solid #16213e; padding-bottom: 0.5rem; }
  h2 { color: #16213e; margin-top: 2rem; }
  .meta { background: #fff; padding: 1rem; border-radius: 6px; border: 1px solid #e0e0e0; margin-bottom: 1.5rem; }
  .meta p { margin: 0.25rem 0; }
  .summary { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 1rem; margin-bottom: 2rem; }
  .stat { background: #fff; padding: 1rem; border-radius: 6px; border: 1px solid #e0e0e0; text-align: center; }
  .stat .value { font-size: 2rem; font-weight: bold; }
  .stat .label { font-size: 0.85rem; color: #666; }
  .stat.pass .value { color: #27ae60; }
  .stat.fail .value { color: #e74c3c; }
  .stat.skip .value { color: #f39c12; }
  .pass { color: #27ae60; font-weight: bold; }
  .fail { color: #e74c3c; font-weight: bold; }
  .skip { color: #f39c12; font-weight: bold; }
  .domain-header { display: flex; align-items: center; gap: 0.5rem; }
  .error { background: #fdf2f2; border: 1px solid #e74c3c; padding: 0.5rem 1rem; border-radius: 4px; margin: 0 0 0.75rem 0; font-size: 0.9rem; }

  .op-list { margin-bottom: 2rem; }
  .op-item { background: #fff; border: 1px solid #e0e0e0; border-radius: 6px; margin-bottom: 0.5rem; overflow: hidden; }
  .op-header { display: flex; align-items: center; padding: 0.75rem 1rem; cursor: pointer; user-select: none; gap: 0.75rem; }
  .op-header:hover { background: #f5f5f5; }
  .op-chevron { transition: transform 0.2s; font-size: 0.7rem; color: #999; flex-shrink: 0; }
  .op-item.open .op-chevron { transform: rotate(90deg); }
  .op-method { font-weight: 700; font-size: 0.8rem; padding: 0.15rem 0.5rem; border-radius: 3px; color: #fff; flex-shrink: 0; min-width: 3.5rem; text-align: center; }
  .op-method.GET { background: #2196f3; }
  .op-method.POST { background: #4caf50; }
  .op-method.PUT { background: #ff9800; }
  .op-method.PATCH { background: #9c27b0; }
  .op-method.DELETE { background: #f44336; }
  .op-name { font-weight: 500; }
  .op-path { color: #666; font-family: monospace; font-size: 0.9rem; }
  .op-meta { margin-left: auto; display: flex; align-items: center; gap: 1rem; flex-shrink: 0; font-size: 0.85rem; }
  .op-duration { color: #888; }
  .op-status { font-weight: 600; }

  .op-detail { display: none; border-top: 1px solid #e0e0e0; padding: 1rem; }
  .op-item.open .op-detail { display: block; }
  .op-detail-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; }
  .op-detail-grid.single { grid-template-columns: 1fr; }
  .op-detail-section h4 { margin: 0 0 0.5rem 0; font-size: 0.85rem; color: #16213e; text-transform: uppercase; letter-spacing: 0.05em; }
  .op-detail-section pre { background: #1e1e2e; color: #cdd6f4; padding: 1rem; border-radius: 4px; overflow-x: auto; font-size: 0.8rem; line-height: 1.5; margin: 0; max-height: 400px; overflow-y: auto; }

  .op-info-row { display: flex; flex-wrap: wrap; gap: 1rem; margin-bottom: 1rem; font-size: 0.85rem; align-items: center; }
  .op-info-item { display: flex; align-items: center; gap: 0.35rem; }
  .op-info-item .label { color: #888; }
  .op-info-item .value { font-weight: 500; }

  .token-wrap { display: inline-flex; align-items: center; gap: 0.35rem; }
  .token-wrap code { font-size: 0.8rem; word-break: break-all; color: #555; max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: inline-block; vertical-align: middle; }
  .small-btn { background: none; border: 1px solid #ccc; border-radius: 3px; cursor: pointer; padding: 0.15rem 0.4rem; font-size: 0.7rem; color: #666; line-height: 1; display: inline-flex; align-items: center; }
  .small-btn:hover { background: #eee; border-color: #999; }
  .small-btn.copied { color: #27ae60; border-color: #27ae60; }

  .raw-overlay { display: none; position: fixed; inset: 0; background: rgba(0,0,0,0.5); z-index: 1000; justify-content: center; align-items: center; }
  .raw-overlay.visible { display: flex; }
  .raw-panel { background: #1e1e2e; color: #cdd6f4; border-radius: 8px; padding: 1.5rem; width: 80vw; max-width: 900px; max-height: 80vh; overflow: auto; position: relative; }
  .raw-panel pre { margin: 0; font-size: 0.8rem; line-height: 1.5; white-space: pre-wrap; word-break: break-word; }
  .raw-close { position: absolute; top: 0.75rem; right: 0.75rem; background: none; border: none; color: #888; font-size: 1.2rem; cursor: pointer; padding: 0.25rem 0.5rem; }
  .raw-close:hover { color: #fff; }
</style>
</head>
<body>
`)

	b.WriteString("<div style=\"margin-bottom:1rem\">")
	b.WriteString(`<svg viewBox="0 0 420 90" fill="none" xmlns="http://www.w3.org/2000/svg" style="height:60px">
  <defs>
    <linearGradient id="railGradLight" x1="10" y1="10" x2="70" y2="70" gradientUnits="userSpaceOnUse">
      <stop offset="0%" stop-color="#00b06a"/>
      <stop offset="100%" stop-color="#0088c0"/>
    </linearGradient>
    <linearGradient id="shieldGradLight" x1="20" y1="5" x2="55" y2="75" gradientUnits="userSpaceOnUse">
      <stop offset="0%" stop-color="#1a2030"/>
      <stop offset="100%" stop-color="#4a5570"/>
    </linearGradient>
    <linearGradient id="restGradLight" x1="90" y1="20" x2="280" y2="65" gradientUnits="userSpaceOnUse">
      <stop offset="0%" stop-color="#1a2030"/>
      <stop offset="100%" stop-color="#3a4560"/>
    </linearGradient>
    <linearGradient id="railWordGradLight" x1="260" y1="20" x2="400" y2="65" gradientUnits="userSpaceOnUse">
      <stop offset="0%" stop-color="#00b06a"/>
      <stop offset="100%" stop-color="#0088c0"/>
    </linearGradient>
  </defs>
  <g transform="translate(8, 6) scale(0.32)">
    <path d="M120 22L42 62V128C42 172 74 210 120 224C166 210 198 172 198 128V62L120 22Z"
          fill="none" stroke="url(#shieldGradLight)" stroke-width="5" stroke-linejoin="round" opacity="0.8"/>
    <path d="M120 38L56 72V128C56 164 82 196 120 208C158 196 184 164 184 128V72L120 38Z"
          fill="none" stroke="url(#railGradLight)" stroke-width="1.5" opacity="0.2"/>
    <line x1="88" y1="70" x2="88" y2="185" stroke="url(#railGradLight)" stroke-width="6" stroke-linecap="round"/>
    <line x1="152" y1="70" x2="152" y2="185" stroke="url(#railGradLight)" stroke-width="6" stroke-linecap="round"/>
    <line x1="82" y1="90" x2="158" y2="90" stroke="url(#railGradLight)" stroke-width="3.5" stroke-linecap="round" opacity="0.6"/>
    <line x1="82" y1="115" x2="158" y2="115" stroke="url(#railGradLight)" stroke-width="3.5" stroke-linecap="round" opacity="0.6"/>
    <line x1="82" y1="140" x2="158" y2="140" stroke="url(#railGradLight)" stroke-width="3.5" stroke-linecap="round" opacity="0.6"/>
    <line x1="82" y1="165" x2="158" y2="165" stroke="url(#railGradLight)" stroke-width="3.5" stroke-linecap="round" opacity="0.6"/>
    <text x="65" y="134" font-family="monospace" font-size="22" font-weight="700" fill="url(#shieldGradLight)" opacity="0.3">&lt;</text>
    <text x="163" y="134" font-family="monospace" font-size="22" font-weight="700" fill="url(#shieldGradLight)" opacity="0.3">&gt;</text>
  </g>
  <text x="88" y="60" font-family="'Outfit', 'Helvetica Neue', Arial, sans-serif" font-weight="800" font-size="48" letter-spacing="-1">
    <tspan fill="url(#restGradLight)">REST</tspan><tspan fill="url(#railWordGradLight)">RAIL</tspan>
  </text>
</svg>`)
	b.WriteString("</div>\n")
	b.WriteString("<h1>Restrail Test Report</h1>\n")
	b.WriteString("<div class=\"meta\">\n")
	b.WriteString(fmt.Sprintf("<p><strong>Timestamp:</strong> %s</p>\n", result.Timestamp.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("<p><strong>Base URL:</strong> %s</p>\n", html.EscapeString(result.BaseURL)))
	b.WriteString(fmt.Sprintf("<p><strong>Profile:</strong> %s</p>\n", html.EscapeString(result.Profile)))
	b.WriteString(fmt.Sprintf("<p><strong>Auth:</strong> %s</p>\n", html.EscapeString(result.AuthType)))
	if result.SpecFile != "" {
		b.WriteString(fmt.Sprintf("<p><strong>Spec:</strong> %s</p>\n", html.EscapeString(result.SpecFile)))
	}
	b.WriteString("</div>\n")

	// Summary
	b.WriteString("<h2>Summary</h2>\n")
	b.WriteString("<div class=\"summary\">\n")
	writeStat(&b, "Total Domains", result.Summary.TotalDomains, "")
	writeStat(&b, "Passed Domains", result.Summary.PassedDomains, "pass")
	writeStat(&b, "Failed Domains", result.Summary.FailedDomains, "fail")
	writeStat(&b, "Total Ops", result.Summary.TotalOps, "")
	writeStat(&b, "Passed Ops", result.Summary.PassedOps, "pass")
	writeStat(&b, "Failed Ops", result.Summary.FailedOps, "fail")
	writeStat(&b, "Skipped Ops", result.Summary.SkippedOps, "skip")
	b.WriteString("</div>\n")

	// Created resources
	if len(result.CreatedResources) > 0 {
		b.WriteString("<h2>Created Resources</h2>\n")
		b.WriteString("<table><tr><th>#</th><th>Domain</th><th>ID Field</th><th>ID</th></tr>\n")
		for i, cr := range result.CreatedResources {
			b.WriteString(fmt.Sprintf("<tr><td>%d</td><td>%s</td><td>%s</td><td><code>%s</code></td></tr>\n",
				i+1, html.EscapeString(cr.Domain), html.EscapeString(cr.IDField), html.EscapeString(cr.ID)))
		}
		b.WriteString("</table>\n")
	}

	// Per domain
	opIdx := 0
	for _, dr := range result.Domains {
		statusClass := "pass"
		statusText := "PASS"
		if !dr.Success {
			statusClass = "fail"
			statusText = "FAIL"
		}
		b.WriteString(fmt.Sprintf("<h2 class=\"domain-header\">Domain: %s <span class=\"%s\">[%s]</span></h2>\n",
			html.EscapeString(dr.Domain), statusClass, statusText))

		// Setup operations
		if len(dr.Setup) > 0 {
			b.WriteString("<details><summary style=\"cursor:pointer;font-weight:600;margin-bottom:0.5rem\">Setup (" + fmt.Sprintf("%d", len(dr.Setup)) + " prerequisite operations)</summary>\n")
			b.WriteString("<table><tr><th>Domain</th><th>Operation</th><th>Method</th><th>Path</th><th>Result</th></tr>\n")
			for _, s := range dr.Setup {
				sClass := "pass"
				sText := "PASS"
				if s.Operation.Skipped {
					sClass = "skip"
					sText = "SKIP"
				} else if !s.Operation.Success {
					sClass = "fail"
					sText = "FAIL"
				}
				b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td class=\"%s\">%s</td></tr>\n",
					html.EscapeString(s.Domain), html.EscapeString(s.Operation.OperationName),
					html.EscapeString(s.Operation.Method), html.EscapeString(s.Operation.Path),
					sClass, sText))
			}
			b.WriteString("</table></details>\n")
		}

		b.WriteString("<div class=\"op-list\">\n")
		for _, op := range dr.Operations {
			writeOperation(&b, &op, opIdx)
			opIdx++
		}
		b.WriteString("</div>\n")
	}

	// Raw overlay container
	b.WriteString("<div class=\"raw-overlay\" id=\"rawOverlay\" onclick=\"if(event.target===this)closeRaw()\">\n")
	b.WriteString("<div class=\"raw-panel\"><button class=\"raw-close\" onclick=\"closeRaw()\">&times;</button><pre id=\"rawContent\"></pre></div>\n")
	b.WriteString("</div>\n")

	// JS
	b.WriteString(`<script>
document.querySelectorAll('.op-header').forEach(function(hdr) {
  hdr.addEventListener('click', function() {
    hdr.parentElement.classList.toggle('open');
  });
});
function copyToken(btn, ev) {
  ev.stopPropagation();
  var token = btn.getAttribute('data-token');
  navigator.clipboard.writeText(token).then(function() {
    btn.textContent = 'copied';
    btn.classList.add('copied');
    setTimeout(function() { btn.textContent = 'copy'; btn.classList.remove('copied'); }, 1500);
  });
}
var rawData = {};
function showRaw(id, ev) {
  ev.stopPropagation();
  var el = document.getElementById('rawContent');
  el.textContent = JSON.stringify(rawData[id], null, 2);
  document.getElementById('rawOverlay').classList.add('visible');
}
function closeRaw() {
  document.getElementById('rawOverlay').classList.remove('visible');
}
document.addEventListener('keydown', function(e) { if (e.key === 'Escape') closeRaw(); });
</script>
`)

	b.WriteString("</body>\n</html>\n")
	return []byte(b.String()), nil
}

func writeOperation(b *strings.Builder, op *runner.OperationResult, idx int) {
	resClass := "pass"
	resText := "PASS"
	if op.Skipped {
		resClass = "skip"
		resText = "SKIP"
	} else if !op.Success {
		resClass = "fail"
		resText = "FAIL"
	}
	duration := time.Duration(op.Duration).Milliseconds()

	// Embed raw JSON data for this operation
	rawJSON := prettyJSON(op)
	b.WriteString(fmt.Sprintf("<script>rawData[%d]=%s;</script>\n", idx, rawJSON))

	b.WriteString("<div class=\"op-item\">\n")

	// Clickable header row
	b.WriteString("<div class=\"op-header\">\n")
	b.WriteString("<span class=\"op-chevron\">&#9654;</span>")
	b.WriteString(fmt.Sprintf("<span class=\"op-method %s\">%s</span>", html.EscapeString(op.Method), html.EscapeString(op.Method)))
	b.WriteString(fmt.Sprintf("<span class=\"op-name\">%s</span>", html.EscapeString(op.OperationName)))
	b.WriteString(fmt.Sprintf("<span class=\"op-path\">%s</span>", html.EscapeString(op.Path)))
	b.WriteString("<span class=\"op-meta\">")
	b.WriteString(fmt.Sprintf("<span class=\"op-duration\">%dms</span>", duration))
	b.WriteString(fmt.Sprintf("<span class=\"op-status %s\">%s</span>", resClass, resText))
	b.WriteString("</span>")
	b.WriteString("</div>\n")

	// Expandable detail
	b.WriteString("<div class=\"op-detail\">\n")

	// Info row
	b.WriteString("<div class=\"op-info-row\">\n")
	if op.Description != "" {
		b.WriteString(fmt.Sprintf("<span class=\"op-info-item\"><span class=\"label\">Description:</span> <span class=\"value\">%s</span></span>", html.EscapeString(op.Description)))
	}
	b.WriteString(fmt.Sprintf("<span class=\"op-info-item\"><span class=\"label\">Expected:</span> <span class=\"value\">%d</span></span>", op.ExpectedStatus))
	b.WriteString(fmt.Sprintf("<span class=\"op-info-item\"><span class=\"label\">Actual:</span> <span class=\"value\">%d</span></span>", op.ActualStatus))
	if op.AuthToken != "" {
		escapedToken := html.EscapeString(op.AuthToken)
		truncated := op.AuthToken
		if len(truncated) > 50 {
			truncated = truncated[:50] + "..."
		}
		b.WriteString(fmt.Sprintf("<span class=\"op-info-item\"><span class=\"label\">Token:</span> <span class=\"token-wrap\"><code title=\"%s\">%s</code><button class=\"small-btn\" data-token=\"%s\" onclick=\"copyToken(this,event)\">copy</button></span></span>",
			escapedToken, html.EscapeString(truncated), escapedToken))
	}
	b.WriteString(fmt.Sprintf("<button class=\"small-btn\" onclick=\"showRaw(%d,event)\">raw</button>", idx))
	b.WriteString("</div>\n")

	if op.Error != "" {
		b.WriteString(fmt.Sprintf("<div class=\"error\"><strong>Error:</strong> %s</div>\n", html.EscapeString(op.Error)))
	}
	if op.SkipReason != "" {
		b.WriteString(fmt.Sprintf("<div class=\"error\"><strong>Skipped:</strong> %s</div>\n", html.EscapeString(op.SkipReason)))
	}

	// Request / Response panels
	hasRequest := op.RequestBody != nil
	hasResponse := op.ResponseBody != nil

	if hasRequest || hasResponse {
		gridClass := "op-detail-grid"
		if hasRequest != hasResponse {
			gridClass += " single"
		}
		b.WriteString(fmt.Sprintf("<div class=\"%s\">\n", gridClass))

		if hasRequest {
			b.WriteString("<div class=\"op-detail-section\">\n")
			b.WriteString("<h4>Request Body</h4>\n")
			b.WriteString(fmt.Sprintf("<pre>%s</pre>\n", html.EscapeString(prettyJSON(op.RequestBody))))
			b.WriteString("</div>\n")
		}
		if hasResponse {
			b.WriteString("<div class=\"op-detail-section\">\n")
			b.WriteString("<h4>Response Body</h4>\n")
			b.WriteString(fmt.Sprintf("<pre>%s</pre>\n", html.EscapeString(prettyJSON(op.ResponseBody))))
			b.WriteString("</div>\n")
		}

		b.WriteString("</div>\n")
	}

	b.WriteString("</div>\n") // op-detail
	b.WriteString("</div>\n") // op-item
}

func prettyJSON(v interface{}) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}

func (r *HTMLReporter) Extension() string {
	return ".html"
}

func formatSecurityHTML(infos []openapi.SecurityInfo) string {
	if len(infos) == 0 {
		return "none"
	}
	var parts []string
	for _, info := range infos {
		label := info.SchemeName + " (" + info.Type
		if info.Scheme != "" {
			label += "/" + info.Scheme
		}
		label += ")"
		if len(info.Scopes) > 0 {
			label += " [" + strings.Join(info.Scopes, ", ") + "]"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, ", ")
}

func writeStat(b *strings.Builder, label string, value int, class string) {
	cls := ""
	if class != "" {
		cls = fmt.Sprintf(" %s", class)
	}
	b.WriteString(fmt.Sprintf("<div class=\"stat%s\"><div class=\"value\">%d</div><div class=\"label\">%s</div></div>\n",
		cls, value, label))
}
