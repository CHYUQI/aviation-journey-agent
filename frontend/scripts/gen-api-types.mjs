#!/usr/bin/env node
// 从 docs/api/openapi.yaml 生成前端类型。
// 契约变更后重新运行：npm run gen:api
//
// 生成结果：frontend/src/types/api.d.ts（自动产物，不要手动编辑）

import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = path.dirname(fileURLToPath(import.meta.url))
const frontendDir = path.resolve(scriptDir, '..')
const spec = path.resolve(frontendDir, '..', 'docs', 'api', 'openapi.yaml')
const outDir = path.resolve(frontendDir, 'src', 'types')
const out = path.join(outDir, 'api.d.ts')

if (!existsSync(spec)) {
  console.error('找不到契约文件：' + spec)
  console.error('请确认 docs/api/openapi.yaml 存在。')
  process.exit(1)
}

// 直接调用包内的 cli.js，避免在 Windows 上 spawn .cmd 触发 EINVAL
const cli = path.join(frontendDir, 'node_modules', 'openapi-typescript', 'bin', 'cli.js')

if (!existsSync(cli)) {
  console.error('缺少依赖 openapi-typescript，请先运行：npm i -D openapi-typescript')
  process.exit(1)
}

mkdirSync(outDir, { recursive: true })

// 用当前 node 运行 cli，任何契约语法错误都会在这里直接失败
execFileSync(process.execPath, [cli, spec, '-o', out], { stdio: 'inherit', cwd: frontendDir })

console.log('已生成 ' + path.relative(frontendDir, out))
