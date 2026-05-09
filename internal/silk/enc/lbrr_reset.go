package enc

import "github.com/rolandhe/silk-go/internal/silk"

// LBRRReset — SKP_Silk_LBRR_reset.
//
// Marks every slot in the LBRR ring buffer as "no LBRR". Called from the
// codec-control path whenever the packet size changes — an LBRR slot
// recorded for the old packet size would be misaligned at the new size.
//
// Translated from vendor/silk/src/SKP_Silk_LBRR_reset.c.
func LBRRReset(c *CommonState) {
	for i := range c.LBRRBuffer {
		c.LBRRBuffer[i].Usage = silk.NoLBRR
	}
}
