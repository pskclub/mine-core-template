import { PrismaClient } from "@prisma/client";

const prisma = new PrismaClient();


async function main() {
  console.log("🌱 Starting seed process...\n");

  try {
    // Seed in order due to foreign key constraints
   

    console.log("\n✨ Seed process completed successfully!");
  } catch (error) {
    console.error("❌ Error during seed process:", error);
    throw error;
  }
}

main()
  .then(async () => {
    await prisma.$disconnect();
  })
  .catch(async (e) => {
    console.error(e);
    await prisma.$disconnect();
    process.exit(1);
  });
