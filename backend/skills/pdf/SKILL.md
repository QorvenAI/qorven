---
name: pdf
description: Use this skill for anything involving PDF files — read, create, merge, split, extract text, add watermarks, fill forms, encrypt/decrypt, OCR.
---

# PDF Skill

Use the `exec` tool to run Python scripts for PDF operations. All scripts are in `{baseDir}/scripts/`.

## Prerequisites

Check Python + dependencies:
```bash
python3 -c "import pypdf; print('pypdf OK')" 2>/dev/null || pip3 install pypdf
python3 -c "import reportlab; print('reportlab OK')" 2>/dev/null || pip3 install reportlab
```

## Operations

### Read/Extract Text
```bash
python3 -c "
from pypdf import PdfReader
reader = PdfReader('INPUT_PATH')
for i, page in enumerate(reader.pages):
    print(f'--- Page {i+1} ---')
    print(page.extract_text())
"
```

### Create PDF from Text
```bash
python3 -c "
from reportlab.lib.pagesizes import letter
from reportlab.pdfgen import canvas
c = canvas.Canvas('OUTPUT_PATH', pagesize=letter)
c.setFont('Helvetica', 12)
y = 750
for line in '''YOUR_TEXT'''.split('\n'):
    c.drawString(72, y, line)
    y -= 15
    if y < 72: c.showPage(); y = 750
c.save()
print('Created OUTPUT_PATH')
"
```

### Merge PDFs
```bash
python3 -c "
from pypdf import PdfMerger
merger = PdfMerger()
for f in ['file1.pdf', 'file2.pdf']:
    merger.append(f)
merger.write('merged.pdf')
merger.close()
print('Merged into merged.pdf')
"
```

### Split PDF
```bash
python3 -c "
from pypdf import PdfReader, PdfWriter
reader = PdfReader('INPUT_PATH')
for i, page in enumerate(reader.pages):
    writer = PdfWriter()
    writer.add_page(page)
    writer.write(f'page_{i+1}.pdf')
print(f'Split into {len(reader.pages)} files')
"
```
