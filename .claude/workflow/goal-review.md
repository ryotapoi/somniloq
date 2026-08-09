# Goal Review

正本は `~/.config/agents/workflow/goal-review.md`。これを Read し、以下の Claude ハーネス固有の reviewer 起動方法を加えて実行する。

## Claude adapter

- reviewer の選択は共通 Goal Review に従い、そこで確定した短名の一覧を増減・置換せず使う。
- 選択された各短名を共通 Model Catalog で family へ解決し、`.claude/workflow/models.md` の Goal Review / Auditor family launchers で launcher へ解決する。解決した launcher に短名と共通 prompt を渡し、fresh な reviewer を1回ずつ実行する。
- 短名、family、launcher のいずれかが未知または利用不能なら停止し、別のモデル、family、launcher で代行しない。
- adapter が結果ファイルを返す場合はOrchestratorが読み、reviewer結果本文を回収する。
- Goal Review 上限時は Goal で確定した `auditors:` を変更せず、`~/.config/agents/workflow/auditor.md` と同じ family launcher 表に従って各 Auditor を1回だけ実行する。
