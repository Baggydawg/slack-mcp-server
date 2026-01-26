# Image Compression Test Harness

This directory is for testing PNG to JPEG compression before integration.

## Usage

1. Build the test tool:
   ```bash
   go build -o compress-test ./cmd/compress-test
   ```

2. Drop PNG screenshots into `input/` directory

3. Run compression test:
   ```bash
   # Test single file
   ./compress-test -input test/compression/input/screenshot.png

   # Test all files in directory
   ./compress-test -input test/compression/input/

   # Specify quality (default: 80)
   ./compress-test -input test/compression/input/ -quality 60
   ```

4. Check `output/` directory for compressed JPEGs

5. Compare sizes and visual quality

## Output Format

```
screenshot.png: 2.1MB -> 847KB (60% reduction) @ quality 80
```
