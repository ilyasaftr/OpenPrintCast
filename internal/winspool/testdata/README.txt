The URF and PWG fixtures were generated with macOS CUPS using generate.c.
Each contains two 4x3 pages, in either sRGB or sGray, at 300 dpi.
The repeated rows and mixed repeated/literal pixels exercise the CUPS
raster encoder rather than a test-only Go encoder.
The large fixtures contain two 2480x3508 sRGB pages, also at 300 dpi.
