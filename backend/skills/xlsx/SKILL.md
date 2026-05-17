---
name: xlsx
description: Use this skill for spreadsheet files — read, create, edit .xlsx/.csv, add formulas, charts, formatting.
---

# XLSX Skill

Use `exec` to run Python with `openpyxl` for spreadsheet operations.

## Prerequisites
```bash
python3 -c "import openpyxl; print('openpyxl OK')" 2>/dev/null || pip3 install openpyxl
```

## Read XLSX
```bash
python3 -c "
from openpyxl import load_workbook
wb = load_workbook('INPUT_PATH')
for sheet in wb.sheetnames:
    ws = wb[sheet]
    print(f'=== Sheet: {sheet} ===')
    for row in ws.iter_rows(values_only=True):
        print(' | '.join(str(c) if c else '' for c in row))
"
```

## Create XLSX
```bash
python3 -c "
from openpyxl import Workbook
from openpyxl.styles import Font, PatternFill
wb = Workbook()
ws = wb.active
ws.title = 'Data'
# Headers
headers = ['Name', 'Value', 'Status']
for i, h in enumerate(headers, 1):
    cell = ws.cell(row=1, column=i, value=h)
    cell.font = Font(bold=True)
    cell.fill = PatternFill('solid', fgColor='4472C4')
# Data rows
data = [['Item 1', 100, 'Active'], ['Item 2', 200, 'Pending']]
for r, row in enumerate(data, 2):
    for c, val in enumerate(row, 1):
        ws.cell(row=r, column=c, value=val)
# Auto-width
for col in ws.columns:
    ws.column_dimensions[col[0].column_letter].width = 15
wb.save('OUTPUT_PATH')
print('Created OUTPUT_PATH')
"
```

## Read CSV
```bash
python3 -c "
import csv
with open('INPUT_PATH') as f:
    reader = csv.reader(f)
    for row in reader:
        print(' | '.join(row))
"
```
