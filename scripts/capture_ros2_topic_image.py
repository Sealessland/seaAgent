#!/usr/bin/env python3
import argparse
import json
import os
import sys
import time


def emit(payload: dict) -> int:
    print(json.dumps(payload, ensure_ascii=False))
    return 0


def import_runtime():
    try:
        import cv2  # type: ignore
        import numpy as np  # type: ignore
        import rclpy  # type: ignore
        from cv_bridge import CvBridge  # type: ignore
        from sensor_msgs.msg import CompressedImage, Image  # type: ignore
    except Exception as exc:
        emit({
            "ok": False,
            "error": f"ros2 python runtime unavailable: {exc}",
        })
        sys.exit(0)
    return cv2, np, rclpy, CvBridge, Image, CompressedImage


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True)
    parser.add_argument("--topic", required=True)
    parser.add_argument("--message-type", required=True)
    parser.add_argument("--encoding", default="bgr8")
    parser.add_argument("--timeout-seconds", type=int, default=5)
    args = parser.parse_args()

    os.makedirs(os.path.dirname(args.output), exist_ok=True)

    cv2, np, rclpy, CvBridge, Image, CompressedImage = import_runtime()

    message_type = args.message_type.strip()
    normalized = message_type.replace("::", "/").replace(".msg.", "/msg/")
    msg_cls = None
    if normalized in ("sensor_msgs/msg/Image", "sensor_msgs/Image"):
        msg_cls = Image
    elif normalized in ("sensor_msgs/msg/CompressedImage", "sensor_msgs/CompressedImage"):
        msg_cls = CompressedImage
    else:
        return emit({
            "ok": False,
            "error": f"unsupported message type: {args.message_type}",
        })

    rclpy.init(args=None)
    bridge = CvBridge()
    node = rclpy.create_node("jetson_ros2_topic_capture")
    received = {"msg": None}

    def callback(msg):
        received["msg"] = msg

    subscription = node.create_subscription(msg_cls, args.topic, callback, 10)
    _ = subscription

    deadline = time.time() + max(args.timeout_seconds, 1)
    try:
        while rclpy.ok() and received["msg"] is None and time.time() < deadline:
            rclpy.spin_once(node, timeout_sec=0.2)

        if received["msg"] is None:
            return emit({
                "ok": False,
                "error": f"timeout waiting for topic {args.topic}",
            })

        msg = received["msg"]
        if isinstance(msg, Image):
            frame = bridge.imgmsg_to_cv2(msg, desired_encoding=args.encoding)
        else:
            array = np.frombuffer(msg.data, dtype=np.uint8)
            frame = cv2.imdecode(array, cv2.IMREAD_COLOR)

        if frame is None:
            return emit({
                "ok": False,
                "error": "failed to decode image payload from ROS2 topic",
            })

        if len(frame.shape) == 3 and frame.shape[2] == 4:
            frame = cv2.cvtColor(frame, cv2.COLOR_BGRA2BGR)

        ok = cv2.imwrite(args.output, frame)
        if not ok:
            return emit({
                "ok": False,
                "error": "failed to write output image",
            })

        return emit({
            "ok": True,
            "output": args.output,
            "width": int(frame.shape[1]),
            "height": int(frame.shape[0]),
            "camera_sn": args.topic,
        })
    finally:
        node.destroy_node()
        rclpy.shutdown()


if __name__ == "__main__":
    sys.exit(main())
