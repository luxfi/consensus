# Lux Consensus White Paper

This directory contains the LaTeX source for the Lux Consensus white paper.

## Structure

- `main.tex` - Main LaTeX document that includes all sections
- `sections/` - Individual section files for easier management:
  - `abstract.tex` - Paper abstract
  - `introduction.tex` - Introduction and motivation
  - `background.tex` - Background and related work
  - `protocol-design.tex` - Core protocol design and algorithms
  - `analysis.tex` - Mathematical analysis and security guarantees
  - `implementation.tex` - Implementation considerations and deployment
  - `conclusion.tex` - Conclusion and future work
- `references.bib` - BibTeX bibliography file
- `figures/` - Directory for figures and diagrams (currently empty)

## Building the Paper

### Prerequisites

You need a LaTeX distribution installed:

**macOS:**
```bash
brew install --cask mactex
```

**Ubuntu/Debian:**
```bash
sudo apt-get install texlive-full
```

**Other systems:** Install TeXLive or MiKTeX

### Build Commands

**Using Make (recommended):**
```bash
# Build the PDF
make paper

# Clean build artifacts
make paper-clean

# Watch for changes and rebuild automatically
make paper-watch
```

**Manual build:**
```bash
cd paper
pdflatex main.tex
bibtex main
pdflatex main.tex
pdflatex main.tex
```

The final PDF will be generated as `paper/main.pdf`.

## Editing Guidelines

- Each section is in its own file for easier collaboration
- Use standard LaTeX formatting and BibTeX for references
- Add new figures to the `figures/` directory
- Update `references.bib` for new citations
- Follow academic paper conventions for structure and tone

## Physics Metaphors

The paper extensively uses physics and astrophysics metaphors to explain the consensus protocol:

- **Photon** - Basic voting messages/interactions
- **Wave** - Threshold voting with interference patterns  
- **Focus** - Confidence accumulation (like laser focusing)
- **Prism** - Conflict set separation
- **Nova** - Linear chain finality (stellar explosion)
- **Nebula** - DAG consensus (star formation)
- **Quasar** - Post-quantum security overlay (cosmic beacons)

These metaphors are not just pedagogical—they guide the actual protocol design and implementation.

## Contributing

When contributing to the paper:

1. Edit the appropriate section file in `sections/`
2. Add new references to `references.bib`
3. Test the build with `make paper`
4. Ensure all citations work properly

The modular structure allows multiple authors to work on different sections simultaneously without conflicts.