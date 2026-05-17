---
name: docx
description: Use this skill for Word documents (.docx) — create, read, edit, format with headings, tables, images, page numbers.
---

# DOCX Skill

Use `exec` to run Python with `python-docx` for Word document operations.

## Prerequisites
```bash
python3 -c "import docx; print('python-docx OK')" 2>/dev/null || pip3 install python-docx
```

## Read DOCX
```bash
python3 -c "
from docx import Document
doc = Document('INPUT_PATH')
for para in doc.paragraphs:
    if para.text.strip():
        print(para.text)
for table in doc.tables:
    for row in table.rows:
        print(' | '.join(cell.text for cell in row.cells))
"
```

## Create DOCX
```bash
python3 -c "
from docx import Document
from docx.shared import Inches, Pt
doc = Document()
doc.add_heading('Title', 0)
doc.add_paragraph('Your content here.')
doc.add_heading('Section', level=1)
doc.add_paragraph('More content.')
# Add table
table = doc.add_table(rows=2, cols=3)
table.style = 'Table Grid'
table.cell(0,0).text = 'Header 1'
table.cell(0,1).text = 'Header 2'
table.cell(0,2).text = 'Header 3'
table.cell(1,0).text = 'Data 1'
table.cell(1,1).text = 'Data 2'
table.cell(1,2).text = 'Data 3'
doc.save('OUTPUT_PATH')
print('Created OUTPUT_PATH')
"
```
