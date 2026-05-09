# silk-go
纯golang实现的rust-silk平替，包括完整的cli功能，及golang api

# 功能
实现与https://lib.rs/crates/rust-silk完全相同能力，纯golang实现，无cgo

## 提供 api接口
可以在golang内直接调用，避免命令行进程调用，silksdk包保留api

## 提供命令行接口
cmd/silk-go是命令行入口，命令行参数与rust版本保持一致
