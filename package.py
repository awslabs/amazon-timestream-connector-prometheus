# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
# the License. A copy of the License is located at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
# CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions

# This script creates precompiled binaries for Linux, Darwin, and Windows and package them into tarballs.
# This script also package the precompiled binary for Linux to a ZIP file that can be uploaded to AWS Lambda as
# the function code.

import argparse
import logging
import os
import platform
import shutil
import subprocess
import tarfile
import time
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


def run_build(target_bin, arch):
    """
    Compiles a binary for the target OS.

    :type target_bin: str
    :param target_bin: The target OS for the binary.
    :type arch: str
    :param arch: The target architecture.
    :return: The name of the binary.
    """
    if not target_bin or not arch:
        logging.error("Target binary or architecture not specified.")
        return None

    # Required for Lambda runtime platform.al2023
    file_name = "bootstrap"

    build_command = f"GOOS={target_bin} GOARCH={arch} go build -o {target_bin}/{file_name}"
    if target_bin == "windows":
        build_command += ".exe"
    logging.debug("Compiling binary for {}-{} with command: {}".format(target_bin, arch, build_command))

    process = subprocess.Popen(build_command, shell=True, stdout=subprocess.PIPE)
    process.communicate()
    return file_name


def check_binary(dir_name, binary):
    """
    Check the directory to make sure the binary has been compiled.

    :param dir_name: The name of the directory.
    :param binary: The name of the compiled binary.
    :return: None
    """
    if dir_name == "windows":
        binary += ".exe"
    check_file(dir_name + "/" + binary)


def check_file(file_name):
    """
    Check whether the file has been created.

    :type file_name: str
    :param file_name: The name of the file to check.
    :return:
    """
    wait_time = 1
    while not os.path.exists(file_name):
        if wait_time >= 256:
            logging.error("Unable to create {file} within the time limit of 256 seconds.".format(file=file_name))
            return

        time.sleep(wait_time)
        wait_time *= 2


def zip_dir(file_name):
    """
    Creates a ZIP file for the binary if target OS is Linux.

    :type file_name: str
    :param file_name: The name of the precompiled binary for Linux.
    """
    logging.debug("Creating a ZIP file for the Linux binary.")
    shutil.make_archive(file_name, 'zip', "linux")


def package_sam_template(linux_bin_name, arch, source_dir, version):
    """
    Package all relevant artifacts for serverless deployment in a tarball.

    :type linux_bin_name: str
    :param linux_bin_name: The name of the precompiled binary for Linux.
    :type arch: str
    :param arch: The target architecture.
    :type source_dir: str
    :param source_dir: The directory containing the SAM template and its documentation.
    :type version: str
    :param version: The artifact version.
    :return: None
    """
    tarfile_name = "timestream-prometheus-connector-serverless-application-{arch}-{version}.tar.gz".format(
        arch=arch, version=version)
    linux_zip = "{file_name}.zip".format(file_name=linux_bin_name)

    with tarfile.open(tarfile_name, "w:gz") as tar:
        for root, dirs, files in os.walk(source_dir):
            for file in files:
                tar.add(os.path.join(root, file), arcname=file)
        check_file(linux_zip)
        tar.add(linux_zip)


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


def create_tarball(target_folder, arch, version):
    """
    Create a tarball containing a precompiled binary and all documentation.

    :type target_folder: str
    :param target_folder: The temporary folder containing the precompiled binary and all documentation.
    :type arch: str
    :param arch: The target architecture.
    :type version: str
    :param version: The version of the Prometheus Connector.
    :return: The name of the precompiled binary.
    """
    create_directory(target_folder)
    bin_name = run_build(target_folder, arch)
    if bin_name is None:
        logging.error("Cannot create binary for packaging.")
        return

    check_binary(target_folder, bin_name)
    archive_name = "timestream-prometheus-connector-{}-{}-{}".format(target_folder, arch, version)
    tar_dir(archive_name, target_folder)
    return archive_name


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-v", "--version", required=True, help="The connector version")
    args = parser.parse_args()

    connector_version = args.version
    logging.basicConfig(level=logging.INFO)
    targets = ["windows", "darwin", "linux"]
    archs = ["amd64", "arm64"]

    try:
        for target in targets:
            for arch in archs:
                bin_name = create_tarball(target, arch, connector_version)
                if target == "linux":
                    zip_dir(bin_name)
                    package_sam_template(bin_name, arch, "./serverless", connector_version)

        logging.info("Done running script.")

    except OSError:
        logging.error("Failed to create a directory for the compiled binary.")
