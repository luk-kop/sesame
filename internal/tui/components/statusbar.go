package components

func Footer(wideMode bool) string {
	wideHint := "w wide"
	if wideMode {
		wideHint = "w default"
	}
	return "↑↓/Pg move  / filter  " + wideHint + "  N/I/S/M/P sort  Enter shell  f tunnel  t tunnels  r refresh  g region  p profile  ? help  q quit"
}
