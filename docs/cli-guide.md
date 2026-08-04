# Hướng Dẫn Chơi Qua CLI

Tài liệu này mô tả cách chạy và chơi bản CLI hiện tại của Tutine TRPG. CLI chỉ là lớp nhập/xuất; phần luật game, memory, orchestration và LLM nằm ở các package `internal/*` để sau này có thể dùng lại cho web hoặc bot.

## Yêu Cầu

- Go 1.26.5 trở lên.
- Terminal hỗ trợ UTF-8 để hiển thị tiếng Việt có dấu.
- API key cho provider trong `configs/llm.yaml`. Mặc định là biến môi trường `GROQ_API_KEY`.

## Chạy Nhanh

Từ thư mục repo:

```bash
export GROQ_API_KEY=your_key_here
go run ./cmd/tu-tien-cli --name Nam
```

TUI mở ở alternate screen với một bố cục có transcript, status và action composer cố định:

```txt
+-- TUTINE TRPG · Ngoại môn Thanh Vân Tông · Lượt 01 ----------------+
| NHẬT KÝ HÀNH TRÌNH                         | NHÂN VẬT               |
| Bạn đứng trước cổng môn, mây phủ lưng núi. | Luyện Khí · tầng 1     |
|                                             | HP 30/30 · Linh lực 20 |
| BẠN MUỐN LÀM GÌ?                                                     |
| 1. Quan sát cổng môn                                                |
| 2. Hỏi đệ tử gác cổng                                               |
| 3. Kiểm tra trạng thái                                              |
| > Bạn muốn làm gì?                                                  |
| Enter gửi · Shift+Enter xuống dòng · Tab gợi ý · / lệnh · Esc thoát|
+---------------------------------------------------------------------+
```

Ở terminal rộng (từ khoảng `100x30`), status nằm ở rail bên phải. Ở narrow layout, status thu thành một dòng; từ 14 hàng trở lên ba gợi ý vẫn là ba dòng riêng. Từ 10–13 hàng chỉ hiển thị gợi ý đang chọn; nhấn `Tab` để đổi gợi ý. Dưới 10 hàng, hãy resize terminal trước khi chơi.

Sau dấu `>`, nhập hành động theo văn tự do hoặc nhập lệnh bắt đầu bằng `/`. `Shift+Enter` thêm dòng mới; draft dài vẫn giữ nguyên trong editor và không đẩy footer khỏi màn hình.

## Tham Số CLI

```bash
go run ./cmd/tu-tien-cli [flags]
```

Các flag hiện có:

| Flag | Mặc định | Ý nghĩa |
| --- | --- | --- |
| `--config` | `configs/llm.yaml` | File cấu hình provider LLM, storage và debug. |
| `--name` | `Vô Danh` | Tên nhân vật người chơi. |

Ví dụ dùng config riêng:

```bash
go run ./cmd/tu-tien-cli --config configs/llm.yaml --name Nam
```

Nếu thiếu `GROQ_API_KEY` hoặc config thiếu trường bắt buộc, CLI sẽ dừng khi khởi động và báo lỗi rõ ràng. CLI không fallback sang fake LLM.

## Lệnh Trong Game

| Lệnh | Tác dụng |
| --- | --- |
| `/help` | Xem danh sách lệnh hiện có. |
| `/status` | Xem trạng thái nhân vật. |
| `/inventory` | Xem túi đồ. |
| `/exit` | Thoát game. |

Các lệnh bắt đầu bằng `/` không tính là một lượt hành động, trừ khi sau này có lệnh được thiết kế riêng để tác động vào game state.

## Nhập Hành Động

Bạn có thể nhập hành động bằng tiếng Việt tự nhiên:

```txt
> ta quan sát cổng môn
```

Engine sẽ xử lý một lượt:

1. Lấy action của người chơi.
2. Gọi planner để chọn tag/entity/keyword cần query.
3. Query SQLite FTS memory.
4. Gọi narrator để tạo lời kể và đề xuất effect.
5. Rule engine validate effect rồi mới cập nhật state.
6. Extract memory mới và lưu kèm tag để các lượt sau query được.

Trong TUI hiện tại, LLM thật xử lý planner, narrator và memory extractor. Tests vẫn dùng fake client, nhưng runtime chơi game không có mode offline.

## Chọn Gợi Ý Bằng Số

Sau một lượt, CLI có thể hiện các lựa chọn:

```txt
1. Quan sát xung quanh
2. Hỏi đệ tử gác cổng
3. Kiểm tra trạng thái
```

Bạn có thể nhập số tương ứng:

```txt
> 1
```

Nếu lựa chọn là hành động roleplay, CLI sẽ gửi nội dung lựa chọn đó vào session như một lượt mới.

Nếu lựa chọn tương ứng với lệnh hệ thống, ví dụ `Kiểm tra trạng thái`, CLI sẽ chạy command nội bộ tương ứng như `/status` và không gọi LLM.

## Ví Dụ Phiên Chơi

1. Chạy `export GROQ_API_KEY=your_key_here`.
2. Chạy `go run ./cmd/tu-tien-cli --name Nam`.
3. Nhập `ta quan sát cổng môn` để bắt đầu lượt chơi.
4. Khi TUI hiện gợi ý, nhập `1`, `2`, `3` để chọn nhanh hoặc nhập hành động tự do.
5. Dùng `/status`, `/inventory`, `/help`, `/exit` khi cần.

TUI không hiển thị ID kỹ thuật của scene, realm hoặc save trong nội dung dành cho người chơi.

## Dữ Liệu Lưu Ở Đâu

Mỗi lượt chơi tạo một save ID riêng và lưu SQLite database trong thư mục cấu hình bởi `storage.data_dir`:

```txt
<data-dir>/saves/<save-id>/game.db
```

Ví dụ với `configs/llm.yaml`, file sẽ nằm dưới:

```txt
./data/dev/saves/<save-id>/game.db
```

Thư mục `data/` là dữ liệu local khi chạy thử, không nên commit vào git.

## Khi Gặp Vấn Đề

Nếu tiếng Việt bị mất dấu hoặc hiển thị sai, kiểm tra terminal đang dùng UTF-8.

Nếu nhập số nhưng không có tác dụng như mong muốn, hãy chắc là trước đó TUI vừa hiện danh sách lựa chọn. Số `1`, `2`, `3` chỉ map theo danh sách gợi ý gần nhất.

Nếu TUI báo lỗi provider, kiểm tra `GROQ_API_KEY`, mạng, `llm.base_url`, và `llm.model` trong `configs/llm.yaml`.
