package autofilter

import (
	"testing"
)

func TestFormatFileButtonText(t *testing.T) {
	// 1. Movie Test
	movieResult := FormatFileButtonText("Dacoit Ek Prem Katha 2026 Hindi 720p.mkv", 1011823084)
	expectedMovie := "[𝟵𝟲𝟰.𝟵𝟱 𝗠𝗕] ➤ 𝗗𝗮𝗰𝗼𝗶𝘁 𝗘𝗸 𝗣𝗿𝗲𝗺 𝗞𝗮𝘁𝗵𝗮 (2026)  𝗛𝗶𝗻𝗱𝗶 • 𝟳𝟮𝟬𝗣"
	if movieResult != expectedMovie {
		t.Errorf("\nExpected movie result:\n%s\nGot:\n%s", expectedMovie, movieResult)
	}

	// 2. Series Test
	seriesResult := FormatFileButtonText("[S01E01] Dark Hole 2026 English 1080p.mkv", 472078336)
	expectedSeries := "[𝟰𝟱𝟬.𝟮𝟭 𝗠𝗕] ➤ [𝗦𝟬𝟭𝗘𝟬𝟭] 𝗗𝗮𝗿𝗸 𝗛𝗼𝗹𝗲 (2026)  𝗘𝗻𝗴𝗹𝗶𝘀𝗵 • 𝟭𝟬𝟴𝟬𝗣"
	if seriesResult != expectedSeries {
		t.Errorf("\nExpected series result:\n%s\nGot:\n%s", expectedSeries, seriesResult)
	}
}
