<p align="right">
  <a href="#中文">中文</a> | <a href="#english">English</a>
</p>

# Case study: making an AI refuse to make things up

<a id="中文"></a>

## 中文

一篇关于 `digital-loved-one` 核心设计决策的简短记录:如何做一个**以"不编造"为立身之本**
的记忆陪伴 AI。

### 背景

这个产品保存一个人的声音、性格和经历,让家人可以和一个有据可循的"人格"对话。这类产品
真正致命的失败模式不是说话生硬 —— 而是**一个自信、编造出来的记忆**:模型凭空发明一段
经历、一个观点、一段关系。对一个和哀思、记忆相关的产品来说,这不是 bug,是对信任的背叛。

所以核心需求和普通聊天机器人是反的:系统必须**更愿意说"我不知道"**,而不是去猜。

### 难点

"不要幻觉"说起来容易,做到很难。Prompt 指令("只用提供的上下文回答")能减少编造,
但**不能保证**—— 一旦检索内容稀薄或来源互相矛盾,LLM 照样会产出流畅、可信、却错误的答案。
我要的是把诚实约束放进**代码**,而不是一句模型随时可能漂移忽略的提示词。

### 决策

**每次推理前都过一道 grounding 闸门。** `grounding.Layer.ValidateForInference` 必须在
任何 LLM 调用之前运行。它检查检索是否充分、暴露未解决的冲突,产出一个 `ValidationResult`。
如果 `Blocked == true`,调用方就**必须**返回"我不知道"—— 生成根本不会发生。诚实保证是一条
**控制流不变量**,不是一句建议。

**证据与解读分离。** 原始素材以逐字的 `SourceExcerpt` 记录存储,关联到 `TopicNode`。
专门的 `conflict` 包负责检测条目之间事实/观点层面的矛盾,并把它**显式暴露出来**,
而不是把两段不相容的记忆悄悄平均成一句圆滑的假话。

**让记忆可审计、可版本化。** 主题节点每次更新都会归档(`_v1`、`_v2`……),所以这个人格的
知识是有历史可查的 —— 你能看到它在什么时候知道了什么,而不是一个不透明的当前状态。

**用接口隔离持久化。** `memory/` 之外的所有代码都只通过 `memory.Store` 接口工作。默认的
`GraphStore` 每个实体写一个 JSON 文件(易读、易 diff、易备份);需要规模时,ObjectBox
引擎藏在一个 build tag 后面。存储选择从不泄漏进分析代码。

**不用框架。** HTTP 层只用 Go 标准库。对一个价值在于数据模型和诚实约束的系统,框架只会
增加表面积,却换不来任何东西。

### 结果

- 一个反幻觉特性被**结构性强制**的 AI 陪伴:证据不足或冲突时,生成被构造性地阻断。
- 干净的关注点分离 —— `ingestion`、`memory`、`grounding`、`conflict`、`inference`,
  每一层都能独立替换。
- 一个持久化边界,让我可以从纯 JSON 文件起步,同时保留切换到真正嵌入式数据库的路径,
  而不用动其余系统。

### 下一步

把 grounding 闸门后的 LLM 生成接通(目前未接通处返回模板回复),并加入检索质量指标,
让"充分性"是被**度量**出来的,而不只是阈值卡出来的。

---

<a id="english"></a>

## English

A short write-up of the core design decision behind `digital-loved-one`: how to
build a memory-companion AI whose defining feature is that it *won't* fabricate.

### Background

The product preserves a person's voice, personality, and history so their family
can talk to a grounded persona. The failure mode that kills this kind of product
is not a clumsy sentence — it is **a confident, fabricated memory**: the model
inventing an event, opinion, or relationship that never existed. For a grief- and
memory-adjacent product, that is not a bug, it is a betrayal of trust.

So the central requirement was inverted from a normal chatbot: the system must be
*willing to say "I don't know"* far more often than it guesses.

### The problem

"Don't hallucinate" is easy to say and hard to enforce. Prompt instructions
("only answer from the provided context") reduce fabrication but do not *guarantee*
it — the moment retrieval is thin or sources disagree, an LLM will still produce a
fluent, plausible, wrong answer. I wanted the honesty constraint to live in code,
not in a prompt that the model can drift away from.

### Decisions

**A grounding gate before every inference.** `grounding.Layer.ValidateForInference`
must run before any LLM call. It checks retrieval sufficiency and surfaces
unresolved conflicts, producing a `ValidationResult`. If `Blocked == true`, the
caller is required to return "I don't know" — generation never happens. The
honesty guarantee is a control-flow invariant, not a suggestion.

**Keep evidence separate from interpretation.** Source material is stored as
verbatim `SourceExcerpt` records linked to `TopicNode`s. A dedicated `conflict`
package detects factual/belief contradictions between excerpts and *surfaces* them
rather than silently averaging two incompatible memories into one smooth lie.

**Make memory auditable and versioned.** Topic nodes are archived on every update
(`_v1`, `_v2`, …), so the persona's knowledge has a history you can inspect — you
can see what it knew and when, instead of an opaque current state.

**Isolate persistence behind an interface.** Everything outside `memory/` works
through a `memory.Store` interface. The default `GraphStore` writes one JSON file
per entity (easy to read, diff, and back up); an ObjectBox engine sits behind a
build tag for when scale matters. Storage choices never leak into analysis code.

**No framework.** The HTTP layer is Go standard library only. For a system whose
value is in its data model and its honesty constraint, a framework would add
surface area without buying anything.

### Result

- An AI companion where the anti-hallucination property is **enforced structurally**:
  insufficient or conflicting evidence blocks generation, by construction.
- Clean separation of concerns — `ingestion`, `memory`, `grounding`, `conflict`,
  `inference` — each replaceable in isolation.
- A persistence boundary that let me start with plain JSON files and keep a path to
  a real embedded database without touching the rest of the system.

### What I'd do next

Wire the gated LLM generation behind the grounding layer (it currently returns
template responses where generation isn't yet connected), and add retrieval-quality
metrics so "sufficiency" is measured, not just thresholded.
