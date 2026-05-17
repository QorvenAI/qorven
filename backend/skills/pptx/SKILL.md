---
name: pptx
description: Use this skill for PowerPoint presentations — create, read, edit .pptx files with slides, layouts, images, charts.
---

# PPTX Skill

Use `exec` to run Python with `python-pptx` for presentation operations.

## Prerequisites
```bash
python3 -c "import pptx; print('python-pptx OK')" 2>/dev/null || pip3 install python-pptx
```

## Read PPTX
```bash
python3 -c "
from pptx import Presentation
prs = Presentation('INPUT_PATH')
for i, slide in enumerate(prs.slides, 1):
    print(f'=== Slide {i} ===')
    for shape in slide.shapes:
        if shape.has_text_frame:
            for para in shape.text_frame.paragraphs:
                if para.text.strip():
                    print(para.text)
"
```

## Create PPTX
```bash
python3 -c "
from pptx import Presentation
from pptx.util import Inches, Pt
prs = Presentation()
# Title slide
slide = prs.slides.add_slide(prs.slide_layouts[0])
slide.shapes.title.text = 'Presentation Title'
slide.placeholders[1].text = 'Subtitle text'
# Content slide
slide = prs.slides.add_slide(prs.slide_layouts[1])
slide.shapes.title.text = 'Slide Title'
body = slide.placeholders[1]
body.text = 'First bullet point'
body.text_frame.add_paragraph().text = 'Second bullet point'
body.text_frame.add_paragraph().text = 'Third bullet point'
prs.save('OUTPUT_PATH')
print('Created OUTPUT_PATH')
"
```
