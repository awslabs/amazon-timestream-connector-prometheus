# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
# the License. A copy of the License is located at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
# CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions

import argparse
import logging
import os
import shutil
import subprocess
import time
import platform
from distutils.dir_util import copy_tree


def create_directory(dir_name):
    """
    Creates a temporary directory to store the compiled binary, and copies the necessary documentation to the directory.

    :type dir_name: str
    :param dir_name: The name of the temporary directory.
    :return: None
    """
    if not os.path.exists(dir_name):
        logging.debug("Creating temporary directory " + dir_name + " for compiled binary.")
        os.mkdir(dir_name)

    logging.debug("Copying README.md, LICENSE, CHANGELOG.md, and documentation to directory " + dir_name)
    for file_name in ["README.md", "LICENSE", "CHANGELOG.md", "GETTING_STARTED.md"]:
        if os.path.isfile(file_name):
            shutil.copy(file_name, dir_name)
    copy_tree("documentation", dir_name + "/documentation")


def run_build(target_bin, connector_version):
    """
    Compiles a binary for the target OS.

    :type target_bin: str
    :param target_bin: The target OS for the binary.
    :return: The name of the binary.
    """
    os.environ['GOOS'] = target_bin
    arch = "amd64"
    os.environ['GOARCH'] = arch

    if os.getenv('GOOS') is None or os.getenv('GOARCH') is None:
        logging.error("Environment variables GOOS or GOARCH are not set.")
        return None

    file_name = "timestream-prometheus-connector-{}-{}-{}".format(target_bin, arch, connector_version)

    build_command = "go build -o {}/{}".format(target_bin, file_name)
    if target_bin == "windows":
        build_command += ".exe"
    logging.debug("Compiling binary for {} with command: {}".format(target_bin, build_command))
    subprocess.Popen(build_command, shell=True, stdout=subprocess.PIPE)
    return file_name


def check_binary(dir_name, bin):
    """
    Check the directory to make sure the binary has been compiled.

    :param dir_name: The name of the directory.
    :param bin: The name fo the compiled binary.
    :return: None
    """
    wait_time = 1
    if dir_name == "windows":
        bin += ".exe"
    while not os.path.exists(dir_name + "/" + bin):
        if wait_time >= 256:
            logging.error("Unable to compile the binary within the time limit of 256 seconds.")
            return

        logging.debug("Waiting for the binary to compile.")
        time.sleep(wait_time)
        wait_time *= 2


def zip_dir(file_name, target_bin):
    """
    Creates a ZIP file for the binary if target OS is Linux.

    :type file_name: str
    :param file_name: The binary name.
    :type target_bin: str
    :param target_bin: The target OS for the binary.
    :return: None
    """
    if target_bin == "linux":
        if platform.system() == "Windows":
            zip_command = "cd linux && tar -a -c -f ../{}.zip * && cd ..".format(file_name)
        else:
            zip_command = "cd linux; zip ../{}.zip *; cd ..".format(file_name)
        logging.debug("Creating a zip file for linux binary")
        subprocess.Popen(zip_command, shell=True, stdout=subprocess.PIPE)


def tar_dir(file_name, dir_name):
    """
    Creates a tarball from the given directory.

    :type file_name: str
    :param file_name: The name of the binary.
    :type dir_name: str
    :param dir_name: The name of the directory.
    :return: None
    """
    tar_command = "tar czf {}.tar.gz {}".format(file_name, dir_name)
    logging.debug("Creating a tarball for " + file_name)
    subprocess.Popen(tar_command, shell=True, stdout=subprocess.PIPE)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-v", "--version", required=True, help="The connector version")
    args = parser.parse_args()

    connector_version = args.version
    logging.basicConfig(level=logging.INFO)
    targets = ["windows", "linux", "darwin"]
    try:
        for target in targets:
            create_directory(target)
            bin_name = run_build(target, connector_version)
            if bin_name is None:
                logging.error("Cannot create binary for packaging.")
                break

            check_binary(target, bin_name)
            zip_dir(bin_name, target)
            tar_dir(bin_name, target)

        logging.info("Done running script.")

    except OSError:
        logging.error("Failed to create a directory for the compiled binary.")
