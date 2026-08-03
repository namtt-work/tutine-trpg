# Tutine TRPG

Tutine TRPG là một game text RPG dùng LLM, lấy cảm hứng từ truyện tu tiên. Game được thiết kế theo hướng luật chơi chặt, game engine viết bằng Go, memory dùng SQLite FTS, và tầng LLM gọi qua các API tương thích OpenAI.

Giao diện chơi đầu tiên sẽ là CLI. CLI chỉ là adapter nhập/xuất: phần session, luật game, memory và orchestration với LLM phải dùng lại được cho web hoặc bot sau này.

## Định Hướng

Tutine là một text RPG hybrid:

- Người chơi có thể chọn tên và một số trait/tính cách nhẹ.
- Campaign ban đầu cố định quanh một tu sĩ mới nhập môn phái.
- Luật game là nguồn sự thật cho tu luyện, combat, inventory, nhiệm vụ, phần thưởng và quan hệ NPC.
- LLM đóng vai trò narrator, NPC actor, retrieval planner và memory extractor.
- SQLite FTS kết hợp tag/entity/fact là lớp memory đầu tiên trước khi thêm vector database.

## Kiến Trúc Dự Kiến

```txt
cmd/tu-tien-cli
  CLI adapter: đọc command, gửi turn vào session layer, render kết quả.

internal/game
  Rule engine và source of truth cho player state, tu luyện, combat, quest, inventory, reward và quan hệ NPC.

internal/orchestrator
  Điều phối turn: retrieval, gọi LLM, validate, cập nhật state, ghi event log và trích xuất memory.

internal/memory
  SQLite FTS store với metadata filter, controlled tag, fact và reranking.

internal/llm
  OpenAI-compatible provider client với JSON/text call, retry, timeout và schema validation.

internal/storage
  Save/load, event log và vòng đời SQLite database.

campaigns/<campaign-id>
  Data pack cho lore, cảnh giới, tag, công pháp, item, NPC, location và quest.
```

## Nguyên Tắc Thiết Kế

- Game state là nguồn sự thật.
- LLM không được sửa state trực tiếp.
- Effect do LLM đề xuất phải được rule engine validate, reject hoặc clamp.
- Campaign tag là controlled vocabulary; tag lạ từ LLM sẽ bị map hoặc bỏ qua.
- Combat outcome do engine resolve trước, LLM chỉ kể lại kết quả.
- Provider config dùng API boundary tương thích OpenAI.

## Trạng Thái Hiện Tại

Foundation CLI đã có thể chạy offline với fake LLM client. Các phần mở rộng như online provider sẽ được bổ sung ở các task tiếp theo.

Design spec:

- [`docs/superpowers/specs/2026-08-03-tu-tien-llm-rpg-design.md`](docs/superpowers/specs/2026-08-03-tu-tien-llm-rpg-design.md)

Setup plan:

- [`docs/superpowers/plans/2026-08-03-tutine-trpg-repo-setup.md`](docs/superpowers/plans/2026-08-03-tutine-trpg-repo-setup.md)

## Chạy Thử Offline

```bash
go run ./cmd/tu-tien-cli --offline --name Nam
```

Offline mode dùng fake LLM client nên không cần API key. Online provider sẽ được nối ở các task sau.

## Tên Gọi

`Tutine` là tên lấy cảm hứng từ cách đảo chữ của `tu tien`/`tu tiên`, nghĩa là tu luyện theo phong cách tiên hiệp/xianxia. Trong project này, `TRPG` được hiểu là text RPG.
