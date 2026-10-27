# Ouroboros 工具链 Go 原生实现路线图

> 范围:`toolchains/*/manifest.yaml`(3 个包)+ `internal/toolchain/appsec_catalog.go`。
> 代码引用格式 `文件:行`,基于 2026-09-15 工作树。

## 0. 落地进度(2026-09/10)

已实现的原生 handler **20 个 operation,覆盖 25 个外部工具 + 新增 hydra 等价**(26 个工具)(`internal/nativetools`):

| 阶段 | 工具 | 状态 |
|---|---|---|
| P1 纯本地 | exiftool(`metadata_extract`)、gitleaks(`secret_scan`)、sslscan(`tls_assessment`)、dnsx(`dns_probe`)、naabu(`port_scan`) | ✅ |
| P2 HTTP | httpx(`http_probe`)、ffuf(`path_fuzz`)、gau+waybackurls(`archive_discovery`)、katana(`crawl`)、arjun(`parameter_discovery`)、subfinder(`subdomain_discovery`) | ✅ |
| P3 子集 | nmap(`service_fingerprint`,connect+banner)、testssl.sh(`tls_assessment`→testssl_json)、trufflehog(`secret_scan`→trufflehog_json)、binwalk(`firmware_analyze`,签名扫描)、dnsrecon(`dns_probe`→dnsrecon_json)、dalfox(`xss_scan`)、sqlmap(`injection_test`)、nuclei(`template_scan`,http 模板子集)、amass(`subdomain_discovery`→amass_json,被动源) | ✅ |
| P4 子集(原"不实现",按子集标准交付) | mobsfscan(`static_code_scan`→mobsfscan_json,8 条移动端规则)、semgrep(`static_code_scan`→semgrep_sarif,规则子集出 SARIF)、apktool(`apk_decompile`→apktool_text,AXML manifest 摘要)、jadx(`apk_decompile`→jadx_metadata_json,APK 清单+dex 方法数) | ✅ |

| 检测能力升级 | nuclei→**通用模板引擎**(template_engine.go:完整 matchers status/word/regex/binary/size/DSL-lite/negative、extractors regex/kval/json/xpath、多请求变量流转、YAML 模板解析、`OURO_TEMPLATES_DIR` 外部模板目录、重定向保留;内置模板 13 个);sqlmap→**五技术全检测**(error/boolean/time-based/UNION 列数/stacked、GET/POST/Cookie 三 place、参数自动发现、19 签名 DBMS 指纹);dalfox→参数自动发现+DOM sink 启发 | ✅ |
| 认证测试(新增 operation) | **hydra 等价 `auth_test`**(`authtest.go`:HTTP Basic/Form 双模式、表单字段发现、哨兵基线差分、锁定检测防账号锁死、`OURO_AUTH_USERWORDS`/`OURO_AUTH_PASSWORDS`/`OURO_AUTH_LOCKOUT_THRESHOLD` 字典与阈值 env、每用户串行+跨用户并发、密码打码 fact) | ✅ |
| 原语深化(2026-10 第二轮,给模型供原材料) | gitleaks 规则 8→**31**(Slack/Stripe/Twilio/OpenAI/私钥/JWT/连接串/熵检);semgrep 规则 12→**28**(命令注入/XXE/SSRF/反序列化/路径拼接/DEBUG 残留);nmap 指纹 24→**91** 条+UDP 顶端口 6 协议(DNS/NTP/SNMP/SSDP/mDNS/memcached)+banner 深度解析(HTTP/SSH/证书 CN);testssl.sh **Heartbleed 真实 TLS record 探测**(手工 ClientHello+RFC6520 扩展)+协议降级实测+弱套件逐个强制;sslscan 全套件枚举(24 个,含 TLS1.3 组探测);katana 补 meta-refresh/HTML 注释/JS 路由拍取;gau/waybackurls 多域名;ffuf 字典 13→**360**+递归一层+403 bypass(X-Original-URL/X-Rewrite-URL);arjun 参数 GET/POST/JSON 三面 | ✅ |
| 观测原语(模型推理路径) | **`http_observe` 新 operation**(`http_observe.go`):调用方指定任意请求形状(method/path/query/headers/body/content-type/baseline),回传全量原始观测(status/headers/时延/body 摘要/表单/链接/JSON 键/基线差分),**零判定**——结论由模型推理产生,防模板固化思维。参数通道闭环:`RunOptions.ToolParameters` → `ToolExecutionRequest.Parameters` → `nativetools.Options.SpecJSON` → dispatch 解析;env 入口 `OURO_HTTP_OBSERVE_SPEC`(JSON) | ✅ |

**26/26 工具全部具备原生 handler(25 个外部工具 + hydra 等价)**。语义边界:内置 13 个高信号
http 模板,外部模板目录(`OURO_TEMPLATES_DIR`)可挂任意 nuclei 格式模板集无限扩展;amass 复用
crt.sh 被动源(图模式/多源仍归 managed);semgrep 是正则规则集(非跨语言 AST/污点);
apktool 不解资源/smali(仅 manifest 摘要,AXML 二进制解析自研);jadx 不反编译 dex→Java
(仅清单与方法数统计);sqlmap 五技术检测不含数据提取/提权;auth_test 不做协议爆破(仅 HTTP
Basic/Form)。深度利用(利用链、数据提取、解包、源码恢复)仍可回退 managed bundle。

补齐的 parser(**此前只有契约、无实现**,证据不落库):`exiftool_json`、`gitleaks_json`、
`testssl_json`、`sslscan_xml`、`trufflehog_json`、`binwalk_text`、
`semgrep_sarif`、`apktool_text`、`jadx_metadata_json`、`auth_test_jsonl`。

开关:`OURO_NATIVE_TOOLS=<tool,tool|all>`,默认空 = 全部走 `managed_binary`(行为可复现)。
共享 operation 按 `Options.ParserID` 分流输出格式(如 `tls_assessment` 视 parser 出 XML 或 JSON,
`dns_probe`/`secret_scan`/`subdomain_discovery`/`static_code_scan`/`apk_decompile` 同理)。
交互盲打(`OURO_INTERACT_URL`,interactsh 风格契约:回调打到 `<base>/<marker>`,`<base>?poll=<marker>` 查询;
模板请求标 `interact: true` 注入 `{{interacturl}}`,发出后 30s 预算内轮询确认,未确认不报告;
SSRF/XXE 内置模板默认静默跳过,配置 env 后生效)。
**观测原语(模型推理路径,2026-10 新方向)**:`http_observe` operation(`internal/nativetools/http_observe.go`)
执行调用方指定的任意 HTTP 请求形状,回传全量原始观测(status/headers/时延/body 摘要/表单/链接/
JSON 键/基线差分),**不做任何判定**——检测结论由模型基于观测推理产生,避免模板固化思维。
参数通道已闭环:`RunOptions.ToolParameters`(Go API)或 `OURO_HTTP_OBSERVE_SPEC`(env,JSON)
→ `ToolExecutionRequest.Parameters` → `nativetools.Options.SpecJSON` → handler;observe 仅接受
url scope target(host/domain 不发错误请求)。
模型可见性:TUI AI 对话的 workspace context 注入 `native_operations.list`(registry 动态,22 个)
与 `http_observe` 用法行,模型能直接看到可推理的 operation 面;观测事实(kind=observation)
经投影层落为 Artifact 证据,链路兼容。
运行参数环境变量(`internal/nativetools/env.go`,调用时求值):`OURO_CT_ENDPOINT`(crt.sh 源)、`OURO_CDX_ENDPOINT`(CDX 源)、`OURO_ARCHIVE_LIMIT`、`OURO_CRAWL_LIMIT`——外部可观测参数一律可覆盖,不硬编码。

**黑板→观测-推理循环架构迁移(2026-10 完成)**:黑板包降级为事实/观测存储基础设施,推理模式
全面切换为观测驱动。核心链路:工具回传原始观测(Observation:kind/target/payload)→ 模型分析
证据形成假设 → 下一观测验证 → 结论落 Fact(带 SourceObservations 证据回链)。落点:
`sisyphus.StepFromObservations`(未处理观测恒优先于假设验证,判定权在模型侧)、AI 对话
`ACTION:OBSERVE`(观测即落 observations.jsonl,最近 5 条注入模型上下文)、API
`GET /api/blackboard/observations`(kind/target/source/q/limit 过滤)、replay 透传
SourceObservations(回放不丢回链)、前端观测-推理工作台(观测时间线/推理流/结论区)。

## 1. 已核实的接缝事实

| 事实 | 位置 |
|---|---|
| 25 个外部二进制 + 1 个规则数据包,全部 `optional: true` | `toolchains/{web-api-pack,infra-pack,ctf-pack}/manifest.yaml` |
| 内置 catalog 26 条(25 `binary` + 1 `rule_bundle`),`AdapterID` 全为 `managed_binary` | `internal/toolchain/appsec_catalog.go:18-59` |
| adapter/operation/SupportedParsers/DependencyNames 契约(18 个 operation) | `internal/toolchain/authored_registry.go:41-170` |
| parser **契约**声明 41 条(≠ 实现) | `internal/toolchain/authored_registry.go:180-222` |
| **唯一** managed_binary 分派点 | `internal/app/run.go:1949-1976` |
| 逐工具 argv 生成 | `internal/app/run.go:2099-2380` |
| parser 派发 switch:**真正实现的只有 15 个 case**,其余 `default: nil` | `internal/app/mobsfscan_parser.go:22-135` |
| 目录/隔离/授权闸门 | `run.go:3101,3119,3596,2691,3709` |
| managed 输出文件 → Transcript 回转 | `run.go:2432` |
| 契约校验:catalog parser 必须 ∈ operation.SupportedParsers | `internal/contracts/contracts.go:127-191` |

已实现的 15 个 parser:`httpx`、`nuclei`、`subfinder`、`ffuf`、`katana`、`url_list`(gau/waybackurls)、`dalfox`、`arjun`、`sqlmap`、`nmap`、`naabu`、`dnsx`、`amass`、`dnsrecon`、`mobsfscan`。

**关键缺口**:`testssl.sh`、`sslscan`、`apktool`、`jadx`、`semgrep`、`gitleaks`、`trufflehog`、`binwalk`、`exiftool` 这 9 个工具既有 parser ID 与 argv,却**没有任何 Go 解析实现**——外部工具跑完,事实不落库。

## 2. 依赖清单

能力缩写:WD=web.discovery、WR=web.route_enumeration、WC=web.candidate_generation、EV=evidence.collection、IH=infra.host_discovery、IS=infra.service_enumeration、II=infra.identity_graph、CA=ctf.artifact_analysis。
"Go parser"列 ✓ 表示有实现文件,✗ 表示仅有契约声明。

| 工具 | 包 | 目标 | operation | parser ID | Go parser | 外部依赖必要性 |
|---|---|---|---|---|---|---|
| nuclei | web-api | url | template_scan | nuclei_jsonl | ✓ | 可移除(模板子集) |
| nuclei-templates | web-api | rule_bundle | — | — | N/A | **必须保留**(规则数据) |
| httpx | web-api | url | http_probe | httpx_jsonl | ✓ | 可移除 |
| subfinder | web-api | domain | subdomain_discovery | subfinder_jsonl | ✓ | 可移除(被动源覆盖下降) |
| ffuf | web-api | url | path_fuzz | ffuf_json | ✓ | 可移除 |
| katana | web-api | url | crawl | katana_jsonl | ✓ | 可移除(无 JS 渲染) |
| gau | web-api | domain | archive_discovery | url_list_text | ✓ | 可移除 |
| waybackurls | web-api | domain | archive_discovery | url_list_text | ✓ | 可移除(与 gau 合并) |
| dalfox | web-api | url | xss_scan | dalfox_json | ✓ | 保留(利用/PoC 面) |
| arjun | web-api | url | parameter_discovery | arjun_json | ✓ | 可移除 |
| sqlmap | web-api | url | injection_test | sqlmap_json | ✓ | **保留**(数据提取) |
| nmap | infra | host | service_fingerprint | nmap_xml | ✓ | 保留(NSE/指纹库) |
| naabu | infra | host | port_scan | naabu_jsonl | ✓ | 可移除(connect 子集) |
| dnsx | infra | domain | dns_probe | dnsx_jsonl | ✓ | 可移除 |
| amass | infra | domain | subdomain_discovery | amass_json | ✓ | 保留(被动源+图) |
| dnsrecon | infra | domain | dns_probe | dnsrecon_json | ✓ | 保留(AXFR/枚举) |
| testssl.sh | infra | host | tls_assessment | testssl_json | ✗ | 保留(漏洞探测) |
| sslscan | infra | host | tls_assessment | sslscan_xml | ✗ | 可移除 |
| apktool | ctf | artifact | apk_decompile | apktool_text | ✗ | **保留**(JVM/aapt2) |
| jadx | ctf | artifact | apk_decompile | jadx_metadata_json | ✗ | **保留**(DEX→Java) |
| mobsfscan | ctf | artifact | static_code_scan | mobsfscan_json | ✓ | 保留(规则更新) |
| semgrep | ctf | artifact | static_code_scan | semgrep_sarif | ✗ | **保留**(多语言 AST/污点) |
| gitleaks | ctf | artifact | secret_scan | gitleaks_json | ✗ | 可移除(内嵌规则) |
| trufflehog | ctf | artifact | secret_scan | trufflehog_json | ✗ | 保留(在线校验) |
| binwalk | ctf | artifact | firmware_analyze | binwalk_text | ✗ | 保留(固件解包) |
| exiftool | ctf | artifact | metadata_extract | exiftool_json | ✗ | 可移除(常见格式子集) |

## 3. 可行性分级

### 一级:可直接 Go 实现(12 个,stdlib 为主)

| 工具 | 理由 |
|---|---|
| httpx | parser 只消费 `url/input`、`status_code`、`content_type`、`content_length`、`final_url`(`httpx_parser.go:162-216`)——一次 HTTP 往返 |
| naabu | TCP connect + CIDR 展开;parser 只读 `ip/host`、`port`、`protocol`(`naabu_parser.go:140-186`) |
| dnsx | 只用 `a/aaaa/cname` + `host`(`dnsx_parser.go:82-128`) |
| subfinder | crt.sh 等公开源 + DNS 枚举,输出 `host`(`subfinder_parser.go:117-122`) |
| ffuf | 字典驱动 HTTP 差分,只读 `url`/`status`(`ffuf_parser.go:197-207`) |
| katana | 静态 HTML 链接抽取(`katana_parser.go:164-221`),无头渲染另计 |
| gau / waybackurls | Internet Archive CDX 查询 → 纯文本 URL 行(`url_list_parser.go:52-84`),两者可合并 |
| arjun | 参数名差分探测(`arjun_parser.go:184-222`) |
| gitleaks | 正则 + 熵的本地扫描 |
| exiftool | JPEG/PNG/GIF/ZIP/PDF 元数据解析 |
| sslscan | `crypto/tls` + `crypto/x509` 枚举协议/套件/证书链 |

### 二级:可交付子集(10 个,需显式标注 subset)

nuclei(http 模板 + matchers/extractors 子集)、nmap(connect + Banner 指纹;SYN/NSE/OS 不现实)、
testssl.sh(配置检查子集,Heartbleed/ROBOT 等需自研)、amass(被动源子集)、dnsrecon(记录枚举子集)、
dalfox(反射型 XSS 子集)、sqlmap(error/boolean/time-based 检测子集,利用仍外部)、mobsfscan(规则子集)、
binwalk(签名扫描子集,解包仍外部)、trufflehog(未验证型 secret 子集)。

### 三级:不现实(3 个,保留 `managed_binary`)

semgrep(跨语言 AST + 污点引擎)、apktool(JVM + baksmali/aapt2 链)、jadx(DEX→Java 反编译)。

新增第三方依赖建议:`golang.org/x/net`(HTML 解析/publicsuffix)、`github.com/miekg/dns`(AXFR/MX/TXT)、
`golang.org/x/crypto`(套件遍历辅助);其余全部 stdlib。

## 4. 推荐实施顺序

| 阶段 | 内容 | 交付判据 |
|---|---|---|
| P0 接缝先行 | 注册 `native_go` adapter + 双跑 golden 框架,先打通 exiftool | 单工具 e2e 通过,parser 零改动 |
| P1 纯本地计算 | exiftool → gitleaks → sslscan → dnsx → naabu | 一级前 5 个可切 native_go |
| P2 HTTP 探测与枚举 | httpx → ffuf → gau/waybackurls → katana → arjun → subfinder | 一级 12 个全部可切 |
| P3 子集语义 | nmap → testssl.sh → dalfox → sqlmap → amass → binwalk → trufflehog → mobsfscan → nuclei | 按子集交付,manifest 标注 subset |
| P4 不实现 | semgrep / apktool / jadx | 维持 managed_binary |

**补漏优先:** 9 个"有契约无解析"的工具(gitleaks/trufflehog/binwalk/exiftool/sslscan/testssl.sh/apktool/jadx/semgrep)
先补 parser,否则无论外部还是原生执行,证据都不会落库。

## 5. 逐工具验收口径

统一兼容性证明:同一 fixture 分别经 (a) 外部二进制、(b) native handler 产出 Transcript(或 managed-outputs 文件),
喂给 `parserEventsForToolExecution`(`internal/app/mobsfscan_parser.go:22`),断言 fact 计数、`kind`、关键字段逐一相等;
golden 文件置 `internal/app/testdata/`。所有 parser 只读 `Transcript` 或 `managedOutputTranscriptForRequest`(`run.go:2432`),
**两者字节兼容即 parser 与事件链零改动**。

| 工具 | 原生输出格式与必含键 | 单测要点 |
|---|---|---|
| httpx | JSONL:`url`、`status_code`、`content_type`、`content_length`、`final_url` | 200/301/404、gzip、超时、出域 URL 拒收 |
| naabu | JSONL:`ip`、`port`、`protocol` | CIDR 展开、端口边界 1/65535、exclusion 拒收 |
| dnsx | JSONL:`host`、`a`/`aaaa`/`cname` | A/AAAA/CNAME、NXDOMAIN、域外 CNAME 拒收 |
| subfinder | JSONL:`host` | 去重、scope 过滤、空结果不报错 |
| ffuf | JSON:`results[].url`、`status` | 字典命中、软 404 拒绝、并发上限 |
| katana | JSONL:`url`、`method`、`depth`、`response.status_code` | 相对链接归一、深度上限、跨域不外拓 |
| gau/waybackurls | 纯文本:每行一个 URL | 非 URL 行忽略、去重、域外丢弃 |
| dalfox | JSON:`type`、`inject_type`、`method`、`param`、`severity`、`data` | 反射 marker 命中/未命中、转义判定 |
| arjun | JSON:`{url: [param...]}` | 参数发现、无参数站点返回空、exclusion 拒收 |
| sqlmap | logFinding:`url`、`parameter`、`place`、`techniques`、`vulnerable` | 可注入/不可注入样本各一、超时降级 |
| nmap | XML:`host/address`、`ports/port/state/service` | 开放/关闭端口、XML 结构对齐(`nmap_parser.go:38-67`) |
| amass | JSON/JSONL:`name`/`domain` | 数组与逐行两种形态、域外过滤 |
| dnsrecon | JSON:`type`、`name`、`address` | A/AAAA/CNAME/MX 对齐 `dnsrecon_parser.go:52-87` |
| testssl.sh / sslscan | JSON/XML:协议、套件、证书链 | 自签证书、仅 TLS1.2、证书过期 |
| mobsfscan | JSON:`results{key:{metadata{...},files[...]}}` | 命中/未命中、file_path 相对化 |
| gitleaks | JSON 数组:`RuleID`、`File`、`StartLine`、`Secret`、`Match` | 命中、无误报、二进制跳过 |
| trufflehog | JSONL:`DetectorName`、`Raw`、`SourceMetadata` | detector 命中、`--no-update` 等价 |
| binwalk | 文本:偏移 + 签名描述行 | 已知固件头命中、无签名返回空 |
| exiftool | JSON 数组:`SourceFile` + 标签键 | JPEG EXIF、PNG chunk、非元数据回退 |
| nuclei | JSONL:`template-id`、`type`、`matched-at`、`info{...}` | 命中/未命中模板、severity 归一、CWE/CVE 透传 |

## 6. 原生执行接缝设计(adapter `native_go`)

1. **注册**:`DefaultAuthoredToolRegistry()`(`authored_registry.go:41`)新增 adapter `native_go`,其每个 operation 的
   `ID`/`RequiredInputs`/`SupportedParsers`/`DependencyNames` 与 `managed_binary` 同名 operation **逐字相同**,
   以保证 `contracts.go:188` 的校验继续通过。
2. **切换粒度**:只改 `appsec_catalog.go` 中该工具的 `BuiltInTool.AdapterID`,逐工具灰度。
3. **分派**:在 `nativeManagedRunCommandWithTarget`(`run.go:1889`)增加 `native_go` 分支:不查 PATH、不 exec,
   直接调用进程内 handler,返回 `ToolExecutionResult{Status:"completed", Transcript:<同格式输出>}`。
4. **输出契约**:handler 必须产出与外部工具字节兼容的 Transcript;文件型输出(nmap_xml、gitleaks_json 等)
   写同一 managed-outputs 路径,使 `managedOutputTranscriptForRequest` 原样回读。
5. **闸门复用(强制)**:native 路径仍走 `requireCommandScopeTarget`、`scopeTargetExcludedForRequest`、
   worker lease 与 `executionIsolationMode`;未注册 adapter fail-closed,落回 offline runner。
6. **共存**:22 个可迁移,3 个保留 `managed_binary`;`nuclei-templates` 是规则数据包,不涉及 adapter。
7. **回退**:`AdapterID` 改回 `managed_binary` 即完成回退(单行)。

## 7. 风险与回退

| 风险 | 缓解 / 回退 |
|---|---|
| 输出漂移导致 parser 静默丢事实 | parser 已做多别名容错;仍以双跑 golden 准入 |
| 子集能力被 planner 高估产生假阴性 | catalog/manifest 显式标注 `native_go` + `subset`,保留外部 fallback |
| 安全语义漂移(native 侧重写 scope) | **禁止重写**,必须复用 `run.go` 既有 helper,单测覆盖出域/exclusion 拒收 |
| 高并发打爆目标或本机 | 复用 `RunTimeoutMS`(`run.go:1848-1860`)与 pack 并发上限 |
| 规则/数据更新通道 | 规则包继续走 managed bundle;内嵌规则版本化并保留 bundle 覆盖入口 |
| 回退成本 | 单行 `AdapterID` 切换,manifest 依赖声明不动 |
