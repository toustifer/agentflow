# Worker Design Rules

这份文档写给 **agentflow 开发者 / 维护者**，不是写给最终用户。

## 1. 控制字段要小而硬

下面这些字段属于系统控制面，应保持有限枚举：
- `kind`
- `launch_mode`
- `escalation_mode`

不要把它们做成任意字符串，否则 leader/runtime 的分支会变得不可预测。

## 2. 策略字段要开放

下面这些字段属于策略面，默认按开放字符串协议处理：
- `task_tags`
- `skills`
- `required_reads`
- `recommended_mcp`
- `fallback_mcp`
- `handoff_targets`
- `recovery_policy`

原则：
- 能注册
- 能持久化
- 能返回
- 能展示
- 未知值不应导致系统崩掉

不要把这些字段轻易收成硬编码枚举。

## 3. 说明字段不要程序化过头

下面这些字段主要给人看：
- `scope`
- `stuck_playbook`

它们应该帮助 worker 和 leader 理解处境，不应该承载复杂控制流。

## 4. 模板字段语法必须稳定

`prompt_template` 可以扩占位符，但模板语法本身不要乱变。

当前约定是：
- 使用 `{task_id}`、`{branch}`、`{worktree_path}` 这类占位符
- 不要混入另一套模板语法，例如 `{{task.id}}`

## 5. 默认不换人

task 一旦交给某个 worker：
- 默认保持 ownership
- 先走 `recovery_policy`
- 再试 `fallback_mcp`
- 最后才按 `escalation_mode` 升级

`reassign` 是例外，不是主路径。

## 6. leader 不负责代做

leader 的职责是：
- 看 phase
- 校验 worker 定义是否完整
- 派发
- 监控
- stuck 时协调

leader 不应该常态化代替 worker 写交付物、commit、submit、pass。

## 7. 新能力先加协议，再加自动执行

如果以后要新增恢复策略，优先顺序是：
1. 先把名字写进 `recovery_policy`
2. 先让系统能存/传/显
3. 确认好用后，再考虑要不要做自动执行逻辑

不要一上来就把每个新策略做成代码分支。

## 8. DB / MCP / runtime 分工

- DB：负责存，不负责理解太多业务
- MCP：负责拦非法输入
- runtime：负责真正执行和解释
- UI/briefing：负责把复杂字段翻译给人看

一句话总结：

> 控制面字段收紧，策略面字段开放，说明面字段保持文本，leader 负责协调不负责代做。
