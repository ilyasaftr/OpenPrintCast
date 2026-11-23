#include <cups/raster.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

int main(int argc, char **argv) {
  if (argc != 4 && argc != 5) return 2;
  int fd = open(argv[2], O_CREAT | O_WRONLY | O_EXCL, 0600);
  if (fd < 0) return 3;
  cups_mode_t mode = strcmp(argv[1], "urf") == 0 ? CUPS_RASTER_WRITE_APPLE : CUPS_RASTER_WRITE_PWG;
  int gray = strcmp(argv[3], "gray") == 0;
  cups_raster_t *r = cupsRasterOpen(fd, mode);
  cups_page_header2_t h;
  memset(&h, 0, sizeof h);
  h.HWResolution[0] = h.HWResolution[1] = 300;
  h.cupsWidth = argc == 5 ? 2480 : 4;
  h.cupsHeight = argc == 5 ? 3508 : 3;
  h.cupsBitsPerColor = 8;
  h.cupsBitsPerPixel = gray ? 8 : 24;
  h.cupsBytesPerLine = h.cupsWidth * (gray ? 1 : 3);
  h.cupsColorOrder = CUPS_ORDER_CHUNKED;
  h.cupsColorSpace = gray ? CUPS_CSPACE_SW : CUPS_CSPACE_SRGB;
  h.cupsNumColors = gray ? 1 : 3;
  h.cupsInteger[0] = 2;
  unsigned char *row = malloc(h.cupsBytesPerLine);
  if (!row) return 6;
  for (int page = 0; page < 2; page++) {
    unsigned char color[] = {255,0,0, 255,0,0, 0,255,0, 0,0,255};
    unsigned char grayrow[] = {0, 0, 128, 255};
    if (page) {
      color[0] = color[3] = 0; color[2] = color[5] = 255;
      grayrow[0] = grayrow[1] = 255;
    }
    if (argc == 5) {
      memset(row, 255, h.cupsBytesPerLine);
      for (unsigned x = 0; x < h.cupsWidth / 2; x++) {
        if (gray) row[x] = page ? 200 : 20;
        else {
          row[3*x] = page ? 0 : 255;
          row[3*x+1] = 0;
          row[3*x+2] = page ? 255 : 0;
        }
      }
    } else {
      memcpy(row, gray ? grayrow : color, h.cupsBytesPerLine);
    }
    if (!cupsRasterWriteHeader2(r, &h)) return 4;
    for (unsigned y = 0; y < h.cupsHeight; y++)
      if (cupsRasterWritePixels(r, row, h.cupsBytesPerLine) != h.cupsBytesPerLine) return 5;
  }
  free(row);
  cupsRasterClose(r);
  close(fd);
  return 0;
}
