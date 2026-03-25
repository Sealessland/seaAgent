#!/usr/bin/env python3
import argparse
import json
import os
import sys
import time

import cv2
import pyzed.sl as sl


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    os.makedirs(os.path.dirname(args.output), exist_ok=True)

    camera = sl.Camera()
    init = sl.InitParameters()
    init.camera_resolution = sl.RESOLUTION.HD720
    init.camera_fps = 15
    init.depth_mode = sl.DEPTH_MODE.NONE

    status = camera.open(init)
    if status != sl.ERROR_CODE.SUCCESS:
        print(json.dumps({
            "ok": False,
            "error": str(status),
            "raw_output": "camera open failed",
        }))
        return 0

    try:
        runtime = sl.RuntimeParameters()
        image = sl.Mat()

        for _ in range(10):
            grab = camera.grab(runtime)
            if grab == sl.ERROR_CODE.SUCCESS:
                camera.retrieve_image(image, sl.VIEW.LEFT)
                frame = image.get_data()
                if frame is None:
                    continue
                if len(frame.shape) == 3 and frame.shape[2] == 4:
                    frame = cv2.cvtColor(frame, cv2.COLOR_BGRA2BGR)
                ok = cv2.imwrite(args.output, frame)
                if not ok:
                    print(json.dumps({
                        "ok": False,
                        "error": "failed to write output image",
                    }))
                    return 0
                info = camera.get_camera_information()
                print(json.dumps({
                    "ok": True,
                    "output": args.output,
                    "width": int(frame.shape[1]),
                    "height": int(frame.shape[0]),
                    "camera_sn": str(info.serial_number),
                }))
                return 0
            time.sleep(0.1)

        print(json.dumps({
            "ok": False,
            "error": "grab did not return a frame",
        }))
        return 0
    finally:
        camera.close()


if __name__ == "__main__":
    sys.exit(main())
