$p = "$PWD\.docxwork\arch.puml"
$t = Get-Content -LiteralPath $p -Raw -Encoding UTF8
# 用行注释替换块注释头，避免 PlantUML 块注释解析问题
$t = $t -replace "(?s)^@startuml\r?\n/'", "@startuml`n'"
$t = $t -replace "'/", "'"
$t = $t -replace "(?m)^\s*'", "'"
# 重新写出
Set-Content -LiteralPath "$PWD\.docxwork\arch2.puml" -Value $t -Encoding UTF8
Get-Content -LiteralPath "$PWD\.docxwork\arch2.puml" -Encoding UTF8 | Select-Object -First 8
